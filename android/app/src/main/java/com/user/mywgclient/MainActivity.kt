package com.user.mywgclient

import android.Manifest
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
            startTunnel()
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
        TunnelController.publishState()
    }

    override fun onDestroy() {
        io.shutdownNow()
        super.onDestroy()
    }

    private fun onConnectClicked() {
        val status = TunnelState.status.value
        if (status == "connected" || status == "connecting") {
            TunnelController.enqueueDisconnect(this)
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
            startTunnel()
        }
    }

    private fun startTunnel() {
        TunnelState.status.value = "connecting"
        TunnelState.message.value = ""
        val text = ConfStore.read(this).orEmpty()
        TunnelController.enqueueConnect(this, text)
    }

    private fun validate(text: String, announce: Boolean): Boolean {
        return when (val inspection = inspectConfig(text)) {
            is ConfigInspection.Rejected -> fail(configFailureText(this, inspection.failure))
            is ConfigInspection.Ready -> {
                if (announce) {
                    ConfStore.write(this, text.trim())
                    val summary = inspection.summary
                    show(
                        getString(
                            R.string.validated,
                            summary.address,
                            summary.endpoint.ifEmpty { "—" },
                            summary.dns.ifEmpty { "—" },
                            summary.routes,
                        ),
                    )
                }
                true
            }
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
