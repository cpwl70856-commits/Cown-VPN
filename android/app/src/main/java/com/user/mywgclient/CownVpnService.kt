package com.user.mywgclient

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.Looper
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import com.wireguard.android.backend.GoBackend

/**
 * [GoBackend.VpnService] plus an ongoing notification. GoBackend still creates the TUN,
 * protects its sockets, and runs wireguard-go. This subclass only owns the foreground
 * notification required while that tunnel is up.
 */
class CownVpnService : GoBackend.VpnService() {
    private val mainHandler = Handler(Looper.getMainLooper())

    @Volatile
    private var destroyed = false

    @Volatile
    private var connectedLabel: String? = null

    override fun onCreate() {
        createNotificationChannel()
        // Promote before super.onCreate() completes GoBackend's service future, so setState
        // cannot establish the TUN before the notification is posted.
        startInForeground(connectedLabel ?: getString(R.string.notif_connecting))
        instance = this
        super.onCreate()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startInForeground(connectedLabel ?: getString(R.string.notif_connecting))
        super.onStartCommand(intent, flags, startId)
        // Always-on is disabled, and a killed process cannot rebuild the tunnel.
        // Do not let the system restart this service into a notification with no tunnel.
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        TunnelState.message.postValue(getString(R.string.vpn_revoked))
        super.onRevoke()
    }

    override fun onDestroy() {
        destroyed = true
        if (instance === this) instance = null
        connectedLabel = null
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        super.onDestroy()
    }

    fun showConnected(address: String) {
        val text = getString(R.string.notif_connected, address)
        connectedLabel = text
        mainHandler.post {
            if (!destroyed) startInForeground(text)
        }
    }

    private fun createNotificationChannel() {
        val manager = getSystemService(NotificationManager::class.java)
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notif_channel),
            NotificationManager.IMPORTANCE_LOW,
        )
        channel.description = getString(R.string.notif_channel_desc)
        channel.setShowBadge(false)
        manager.createNotificationChannel(channel)
    }

    private fun startInForeground(text: String) {
        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_stat_vpn)
            .setContentIntent(contentIntent())
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
        val foregroundType = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED
        } else {
            0
        }
        ServiceCompat.startForeground(this, NOTIF_ID, notification, foregroundType)
    }

    private fun contentIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        return PendingIntent.getActivity(this, 0, intent, flags)
    }

    companion object {
        private const val CHANNEL_ID = "cown_vpn"
        private const val NOTIF_ID = 1

        @Volatile
        private var instance: CownVpnService? = null

        fun showConnected(address: String) {
            instance?.showConnected(address)
        }
    }
}
