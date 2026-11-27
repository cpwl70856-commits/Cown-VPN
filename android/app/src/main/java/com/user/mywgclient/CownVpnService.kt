package com.user.mywgclient

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.core.app.NotificationCompat
import wgmobile.Protector
import wgmobile.Wgmobile
import java.util.concurrent.Executors

/**
 * Owns the Android TUN. Go (wgmobile) runs WireGuard on a duplicated fd and does
 * not install routes; this service does that with [VpnService.Builder].
 * The original fd is closed after [Wgmobile.startWithFD] returns.
 */
class CownVpnService : VpnService() {
    private val handler = Handler(Looper.getMainLooper())
    private val executor = Executors.newSingleThreadExecutor { runnable ->
        Thread(runnable, "cown-vpn")
    }
    private val opLock = Any()
    private var generation = 0

    @Volatile
    private var destroyed = false

    @Volatile
    private var polling = false

    private val protector = Protector { fd ->
        val ok = protect(fd.toInt())
        if (!ok) Log.w(TAG, "VpnService.protect returned false")
        ok
    }

    private val reprotect = Runnable {
        try {
            protectSockets()
        } catch (e: Exception) {
            Log.w(TAG, "reprotect failed", e)
        }
    }

    private val statsTask = object : Runnable {
        override fun run() {
            if (!polling) return
            try {
                TunnelState.stats.postValue(formatStats(Wgmobile.statsJSON()))
            } catch (e: Exception) {
                Log.w(TAG, "stats failed", e)
            }
            if (polling) handler.postDelayed(this, 2000)
        }
    }

    override fun onCreate() {
        super.onCreate()
        val manager = getSystemService(NotificationManager::class.java)
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notif_channel),
            NotificationManager.IMPORTANCE_DEFAULT,
        )
        channel.description = getString(R.string.notif_channel_desc)
        manager.createNotificationChannel(channel)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_DISCONNECT -> requestDisconnect()
            else -> requestConnect()
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        beginOp()
        executor.execute {
            synchronized(opLock) {
                teardownLocked()
                TunnelState.message.postValue(getString(R.string.vpn_revoked))
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        }
    }

    override fun onDestroy() {
        destroyed = true
        handler.removeCallbacksAndMessages(null)
        synchronized(opLock) {
            teardownLocked()
        }
        executor.shutdownNow()
        super.onDestroy()
    }

    private fun requestConnect() {
        TunnelState.status.postValue("connecting")
        try {
            ensureForeground(getString(R.string.notif_connecting))
        } catch (e: Exception) {
            TunnelState.status.postValue("disconnected")
            TunnelState.message.postValue(getString(R.string.connect_failed, e.message ?: e.javaClass.simpleName))
            stopSelf()
            return
        }
        val gen = beginOp()
        executor.execute {
            synchronized(opLock) {
                if (gen != generation || destroyed) return@execute
                try {
                    bringUp()
                } catch (e: Exception) {
                    Log.w(TAG, "connect failed", e)
                    postFail(getString(R.string.connect_failed, e.message ?: e.javaClass.simpleName))
                }
            }
        }
    }

    private fun requestDisconnect() {
        beginOp()
        executor.execute {
            synchronized(opLock) {
                teardownLocked()
                TunnelState.message.postValue("")
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        }
    }

    private fun bringUp() {
        if (destroyed) return
        val conf = ConfStore.read(this)?.trim().orEmpty()
        if (conf.isEmpty()) {
            postFail(getString(R.string.err_empty))
            return
        }
        val hints = tunnelHints(Wgmobile.parseConf(conf))
        val address = hints.address
        if (address == null) {
            postFail(getString(R.string.err_address))
            return
        }
        val builder = Builder()
            .setSession(getString(R.string.app_name))
            .setMtu(hints.mtu)
            .setConfigureIntent(contentIntent())
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            builder.setMetered(false)
        }
        try {
            builder.addAddress(address.host, address.prefix)
        } catch (e: IllegalArgumentException) {
            postFail(getString(R.string.err_address))
            return
        }
        for (dns in hints.dns) {
            try {
                builder.addDnsServer(dns)
            } catch (_: IllegalArgumentException) {
                Log.w(TAG, "skipping unusable dns server")
            }
        }
        var routes = 0
        for (route in hints.routes) {
            try {
                builder.addRoute(route.host, route.prefix)
                routes++
            } catch (_: IllegalArgumentException) {
                Log.w(TAG, "skipping unusable route")
            }
        }
        if (routes == 0) {
            postFail(getString(R.string.err_no_route))
            return
        }
        if (destroyed) return
        val pfd = builder.establish()
        if (pfd == null) {
            postFail(getString(R.string.vpn_denied))
            return
        }
        try {
            Wgmobile.setProtector(protector)
            pfd.use { parcel ->
                Wgmobile.startWithFD(conf, parcel.fd.toLong())
            }
        } catch (e: Exception) {
            try {
                Wgmobile.stop()
            } catch (stopError: Exception) {
                Log.w(TAG, "stop after failed start", stopError)
            }
            try {
                Wgmobile.clearProtector()
            } catch (clearError: Exception) {
                Log.w(TAG, "clearProtector after failed start", clearError)
            }
            postFail(getString(R.string.connect_failed, e.message ?: e.javaClass.simpleName))
            return
        }
        if (destroyed) {
            teardownLocked()
            return
        }
        protectSockets()
        markConnected(address.toString())
    }

    private fun protectSockets() {
        val v4 = Wgmobile.socketV4()
        if (v4 >= 0 && !protect(v4.toInt())) {
            Log.w(TAG, "protect v4 returned false")
        }
        val v6 = Wgmobile.socketV6()
        if (v6 >= 0 && !protect(v6.toInt())) {
            Log.w(TAG, "protect v6 returned false")
        }
    }

    private fun markConnected(address: String) {
        TunnelState.status.postValue("connected")
        TunnelState.message.postValue("")
        val manager = getSystemService(NotificationManager::class.java)
        manager.notify(NOTIF_ID, buildNotification(getString(R.string.notif_connected, address)))
        polling = true
        handler.post(statsTask)
        handler.post(reprotect)
        handler.postDelayed(reprotect, 1_000)
    }

    private fun postFail(message: String) {
        teardownLocked()
        TunnelState.status.postValue("disconnected")
        TunnelState.message.postValue(message)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun teardownLocked() {
        polling = false
        handler.removeCallbacks(reprotect)
        handler.removeCallbacks(statsTask)
        try {
            Wgmobile.stop()
        } catch (e: Exception) {
            Log.w(TAG, "stop failed", e)
        }
        try {
            Wgmobile.clearProtector()
        } catch (e: Exception) {
            Log.w(TAG, "clearProtector failed", e)
        }
        TunnelState.status.postValue("disconnected")
        TunnelState.stats.postValue("")
    }

    private fun ensureForeground(text: String) {
        val notification = buildNotification(text)
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIF_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, notification)
        }
    }

    private fun buildNotification(text: String) = NotificationCompat.Builder(this, CHANNEL_ID)
        .setContentTitle(getString(R.string.app_name))
        .setContentText(text)
        .setSmallIcon(R.drawable.ic_stat_vpn)
        .setContentIntent(contentIntent())
        .setOngoing(true)
        .setOnlyAlertOnce(true)
        .setCategory(NotificationCompat.CATEGORY_SERVICE)
        .build()

    private fun contentIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java)
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        return PendingIntent.getActivity(this, 0, intent, flags)
    }

    private fun beginOp(): Int {
        synchronized(opLock) {
            generation += 1
            return generation
        }
    }

    companion object {
        const val ACTION_CONNECT = "com.user.mywgclient.CONNECT"
        const val ACTION_DISCONNECT = "com.user.mywgclient.DISCONNECT"
        private const val CHANNEL_ID = "cown_vpn"
        private const val NOTIF_ID = 1
        private const val TAG = "CownVpn"
    }
}
