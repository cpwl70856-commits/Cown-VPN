package com.user.mywgclient

import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import android.util.Log
import com.wireguard.android.backend.GoBackend
import com.wireguard.android.backend.Tunnel
import java.util.concurrent.Executors

/**
 * Starts and stops the tunnel with [GoBackend.setState]. One backend per process: the
 * library keeps the running VPN service in a static future.
 */
internal object TunnelController {
    private const val TAG = "CownVpn"
    private val io = Executors.newSingleThreadExecutor { runnable -> Thread(runnable, "cown-tunnel") }
    private val mainHandler = Handler(Looper.getMainLooper())
    private val tunnel = CownTunnel()

    @Volatile
    private var backend: GoBackend? = null

    @Volatile
    private var polling = false

    private val statsTask = object : Runnable {
        override fun run() {
            if (!polling) return
            val current = backend
            if (current != null) {
                try {
                    val stats = current.getStatistics(tunnel)
                    var handshake = 0L
                    for (key in stats.peers()) {
                        val at = stats.peer(key)?.latestHandshakeEpochMillis ?: 0L
                        if (at > handshake) handshake = at
                    }
                    TunnelState.stats.postValue(formatTunnelStats(stats.totalRx(), stats.totalTx(), handshake))
                } catch (error: Exception) {
                    Log.w(TAG, "stats failed", error)
                }
            }
            if (polling) mainHandler.postDelayed(this, 2_000)
        }
    }

    fun enqueueConnect(context: Context, confText: String) {
        val app = context.applicationContext
        io.execute {
            when (val inspection = inspectConfig(confText)) {
                is ConfigInspection.Rejected -> fail(app, configFailureText(app, inspection.failure))
                is ConfigInspection.Ready -> bringUp(app, inspection.summary)
            }
        }
    }

    fun enqueueDisconnect(context: Context) {
        val app = context.applicationContext
        io.execute {
            try {
                bringDown(app)
                TunnelState.message.postValue("")
            } catch (error: Exception) {
                Log.w(TAG, "disconnect failed: ${error.javaClass.simpleName}")
                TunnelState.message.postValue(startFailureText(app, error))
            }
        }
    }

    fun publishState() {
        val current = backend ?: return
        val up = current.getState(tunnel) == Tunnel.State.UP
        val shown = TunnelState.status.value
        if (up) {
            TunnelState.status.postValue("connected")
            onUp()
        } else if (shown != "connecting") {
            TunnelState.status.postValue("disconnected")
            onDown()
        }
    }

    private fun bringUp(context: Context, summary: ConfigSummary) {
        try {
            val state = backend(context).setState(tunnel, Tunnel.State.UP, summary.config)
            if (state != Tunnel.State.UP) {
                throw IllegalStateException("隧道未进入连接状态")
            }
            CownVpnService.showConnected(summary.address)
            TunnelState.message.postValue("")
        } catch (error: Throwable) {
            Log.w(TAG, "connect failed: ${error.javaClass.simpleName}")
            abandon(context)
            fail(context, startFailureText(context, error))
        }
    }

    private fun bringDown(context: Context) {
        onDown()
        val current = backend
        if (current != null && current.getState(tunnel) == Tunnel.State.UP) {
            current.setState(tunnel, Tunnel.State.DOWN, null)
        }
        context.stopService(Intent(context, CownVpnService::class.java))
        TunnelState.status.postValue("disconnected")
        TunnelState.stats.postValue("")
    }

    private fun abandon(context: Context) {
        try {
            bringDown(context)
        } catch (error: Throwable) {
            Log.w(TAG, "cleanup after failed start", error)
            context.stopService(Intent(context, CownVpnService::class.java))
        }
    }

    private fun fail(context: Context, message: String) {
        onDown()
        TunnelState.status.postValue("disconnected")
        TunnelState.stats.postValue("")
        TunnelState.message.postValue(message)
        context.stopService(Intent(context, CownVpnService::class.java))
    }

    private fun backend(context: Context): GoBackend {
        backend?.let { return it }
        return synchronized(this) {
            backend ?: GoBackend(WireGuardBackendContext(context)).also { backend = it }
        }
    }

    private fun onUp() {
        polling = true
        mainHandler.removeCallbacks(statsTask)
        mainHandler.post(statsTask)
    }

    private fun onDown() {
        polling = false
        mainHandler.removeCallbacks(statsTask)
    }

    private class CownTunnel : Tunnel {
        override fun getName(): String = "cown"

        override fun onStateChange(newState: Tunnel.State) {
            if (newState == Tunnel.State.UP) {
                TunnelState.status.postValue("connected")
                onUp()
            } else {
                TunnelState.status.postValue("disconnected")
                TunnelState.stats.postValue("")
                onDown()
            }
        }
    }
}
