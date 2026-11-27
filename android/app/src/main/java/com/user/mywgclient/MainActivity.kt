package com.user.mywgclient

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import com.user.mywgclient.databinding.ActivityMainBinding
import wgmobile.Wgmobile
import java.util.concurrent.Executors

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding
    private val io = Executors.newSingleThreadExecutor()
    private var pendingConnect = false

    private val openConf = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null) return@registerForActivityResult
        io.execute {
            try {
                val text = contentResolver.openInputStream(uri)?.use { input ->
                    input.bufferedReader().readText()
                }.orEmpty()
                runOnUiThread {
                    if (isDestroyed) return@runOnUiThread
                    if (text.length > MAX_CONF_CHARS) {
                        show(getString(R.string.import_too_large))
                        return@runOnUiThread
                    }
                    binding.conf.setText(text)
                    if (text.isNotBlank()) ConfStore.write(this, text)
                    show(getString(R.string.imported))
                }
            } catch (e: Exception) {
                runOnUiThread {
                    if (!isDestroyed) show(getString(R.string.import_failed, e.message ?: ""))
                }
            }
        }
    }

    private val notifPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (!granted) show(getString(R.string.notif_denied))
        if (pendingConnect) {
            pendingConnect = false
            prepareVpn()
        }
    }

    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == RESULT_OK) {
            startTunnelService()
        } else {
            show(getString(R.string.vpn_denied))
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)
        ViewCompat.setOnApplyWindowInsetsListener(binding.root) { view, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime(),
            )
            val extra = (16 * resources.displayMetrics.density).toInt()
            view.setPadding(bars.left + extra, bars.top + extra, bars.right + extra, bars.bottom + extra)
            WindowInsetsCompat.CONSUMED
        }

        val saved = ConfStore.read(this)
        binding.conf.setText(if (saved.isNullOrBlank()) PLACEHOLDER else saved)

        TunnelState.status.observe(this) { status ->
            binding.status.text = when (status) {
                "connected" -> getString(R.string.status_connected)
                "connecting" -> getString(R.string.status_connecting)
                else -> getString(R.string.status_disconnected)
            }
            binding.connect.text = if (status == "connected" || status == "connecting") {
                getString(R.string.action_disconnect)
            } else {
                getString(R.string.action_connect)
            }
        }
        TunnelState.stats.observe(this) { binding.stats.text = it.orEmpty() }
        TunnelState.message.observe(this) { binding.message.text = it.orEmpty() }

        binding.importConf.setOnClickListener {
            openConf.launch(arrayOf("text/*", "application/octet-stream", "*/*"))
        }
        binding.validateConf.setOnClickListener {
            validate(binding.conf.text?.toString().orEmpty(), announce = true)
        }
        binding.connect.setOnClickListener { onConnectClicked() }
    }

    override fun onResume() {
        super.onResume()
        try {
            val go = Wgmobile.status()
            val shown = TunnelState.status.value
            if (go == "connected" || (go == "disconnected" && shown != "connecting")) {
                TunnelState.status.value = go
            }
        } catch (_: Exception) {
            // The AAR is present whenever this activity compiles.
        }
    }

    override fun onDestroy() {
        io.shutdownNow()
        super.onDestroy()
    }

    private fun onConnectClicked() {
        val status = TunnelState.status.value
        if (status == "connected" || status == "connecting") {
            startService(
                Intent(this, CownVpnService::class.java).setAction(CownVpnService.ACTION_DISCONNECT),
            )
            return
        }
        val text = binding.conf.text?.toString().orEmpty()
        if (!validate(text, announce = false)) return
        ConfStore.write(this, text.trim())
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            pendingConnect = true
            notifPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
            return
        }
        prepareVpn()
    }

    private fun prepareVpn() {
        val intent = VpnService.prepare(this)
        if (intent != null) {
            vpnConsent.launch(intent)
        } else {
            startTunnelService()
        }
    }

    private fun startTunnelService() {
        ContextCompat.startForegroundService(
            this,
            Intent(this, CownVpnService::class.java).setAction(CownVpnService.ACTION_CONNECT),
        )
    }

    private fun validate(text: String, announce: Boolean): Boolean {
        val trimmed = text.trim()
        if (trimmed.isEmpty()) return fail(getString(R.string.err_empty))
        if (trimmed.length > MAX_CONF_CHARS) return fail(getString(R.string.import_too_large))
        return try {
            val hints = tunnelHints(Wgmobile.parseConf(trimmed))
            // Discard the UAPI string. It contains key material; the call only checks that keys parse.
            check(Wgmobile.buildUAPI(trimmed).isNotEmpty())
            val address = hints.address ?: return fail(getString(R.string.err_address))
            if (hints.routes.isEmpty()) return fail(getString(R.string.err_no_route))
            if (announce) {
                ConfStore.write(this, trimmed)
                show(
                    getString(
                        R.string.validated,
                        address.toString(),
                        hints.endpoint.ifEmpty { "—" },
                        if (hints.dns.isEmpty()) "—" else hints.dns.joinToString(", "),
                        hints.routes.joinToString(", "),
                    ),
                )
            }
            true
        } catch (e: Exception) {
            fail(getString(R.string.err_invalid, e.message ?: e.javaClass.simpleName))
        }
    }

    private fun fail(message: String): Boolean {
        show(message)
        return false
    }

    private fun show(message: String) {
        TunnelState.message.value = message
    }

    companion object {
        private const val MAX_CONF_CHARS = 256 * 1024
        private val PLACEHOLDER = """
            [Interface]
            PrivateKey = REPLACE_CLIENT_PRIVATE_KEY
            Address = 10.8.0.2/24
            DNS = 1.1.1.1

            [Peer]
            PublicKey = REPLACE_SERVER_PUBLIC_KEY
            Endpoint = vpn.example.com:51820
            AllowedIPs = 0.0.0.0/0
            PersistentKeepalive = 25
        """.trimIndent()
    }
}
