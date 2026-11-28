package com.user.mywgclient

import com.wireguard.android.backend.BackendException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ConfigInspectionTest {
    @Test
    fun emptyConfigIsRejected() {
        val result = inspectConfig("  \n")
        assertTrue(result is ConfigInspection.Rejected)
        assertEquals(ConfigFailure.Empty, (result as ConfigInspection.Rejected).failure)
    }

    @Test
    fun oversizedConfigIsRejected() {
        val result = inspectConfig("a".repeat(MAX_CONF_CHARS + 1))
        assertEquals(ConfigFailure.TooLarge, (result as ConfigInspection.Rejected).failure)
    }

    @Test
    fun missingPrivateKeyIsChinese() {
        val result = inspectConfig(
            """
            [Interface]
            Address = 10.8.0.2/24

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            AllowedIPs = 0.0.0.0/0
            """.trimIndent(),
        )
        val failure = (result as ConfigInspection.Rejected).failure as ConfigFailure.Invalid
        assertTrue(failure.detail.contains("缺少字段"))
        assertTrue(failure.detail.contains("PrivateKey"))
    }

    @Test
    fun missingAddressIsRejected() {
        val result = inspectConfig(
            """
            [Interface]
            PrivateKey = $CLIENT_PRIVATE_KEY

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            Endpoint = vpn.example.com:51820
            AllowedIPs = 0.0.0.0/0
            """.trimIndent(),
        )
        assertEquals(ConfigFailure.NoAddress, (result as ConfigInspection.Rejected).failure)
    }

    @Test
    fun missingAllowedIpsIsRejected() {
        val result = inspectConfig(
            """
            [Interface]
            PrivateKey = $CLIENT_PRIVATE_KEY
            Address = 10.8.0.2/24

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            Endpoint = vpn.example.com:51820
            """.trimIndent(),
        )
        assertEquals(ConfigFailure.NoRoute, (result as ConfigInspection.Rejected).failure)
    }

    @Test
    fun invalidKeyIsChinese() {
        val result = inspectConfig(
            """
            [Interface]
            PrivateKey = not-a-key
            Address = 10.8.0.2/24

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            AllowedIPs = 0.0.0.0/0
            """.trimIndent(),
        )
        val failure = (result as ConfigInspection.Rejected).failure as ConfigFailure.Invalid
        assertTrue(failure.detail.contains("密钥无效"))
    }

    @Test
    fun unknownSectionIsChinese() {
        val result = inspectConfig("this is not a wireguard config")
        val failure = (result as ConfigInspection.Rejected).failure as ConfigFailure.Invalid
        assertTrue(failure.detail.contains("未知段落"))
    }

    @Test
    fun validConfigKeepsImportFields() {
        val result = inspectConfig(VALID)
        val summary = (result as ConfigInspection.Ready).summary
        assertEquals("10.8.0.2/24", summary.address)
        assertEquals("vpn.example.com:51820", summary.endpoint)
        assertEquals("1.1.1.1", summary.dns)
        assertEquals("0.0.0.0/0", summary.routes)
    }

    @Test
    fun commentsAndBlankLinesStillParse() {
        val text = """
            # imported from a file

            [Interface]
            PrivateKey = $CLIENT_PRIVATE_KEY
            Address = 10.8.0.2/24

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            Endpoint = vpn.example.com:51820
            AllowedIPs = 0.0.0.0/0
        """.trimIndent()
        assertTrue(inspectConfig(text) is ConfigInspection.Ready)
    }

    @Test
    fun startFailuresMapToStableReasons() {
        assertEquals(
            StartFailure.MissingConfig,
            classifyStartFailure(BackendException(BackendException.Reason.TUNNEL_MISSING_CONFIG)),
        )
        assertEquals(
            StartFailure.VpnNotAuthorized,
            classifyStartFailure(BackendException(BackendException.Reason.VPN_NOT_AUTHORIZED)),
        )
        assertEquals(
            StartFailure.Dns("vpn.example.com"),
            classifyStartFailure(
                BackendException(BackendException.Reason.DNS_RESOLUTION_FAILURE, "vpn.example.com"),
            ),
        )
        val tun = classifyStartFailure(
            BackendException(BackendException.Reason.TUN_CREATION_ERROR),
        ) as StartFailure.Other
        assertTrue(tun.detail.contains("无法创建 VPN 接口"))
        val activation = classifyStartFailure(
            BackendException(BackendException.Reason.GO_ACTIVATION_ERROR_CODE, -1),
        ) as StartFailure.Other
        assertTrue(activation.detail.contains("隧道启动失败"))
        assertTrue(activation.detail.contains("-1"))
        val other = classifyStartFailure(IllegalStateException("boom")) as StartFailure.Other
        assertEquals("boom", other.detail)
    }

    @Test
    fun statsLineShowsHandshake() {
        assertEquals("connected  ↓1  ↑2  已握手", formatTunnelStats(1, 2, 5))
        assertEquals("connected  ↓0  ↑0  尚无握手", formatTunnelStats(0, 0, 0))
    }

    companion object {
        private const val CLIENT_PRIVATE_KEY = "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
        private const val SERVER_PUBLIC_KEY = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
        private val VALID = """
            [Interface]
            PrivateKey = $CLIENT_PRIVATE_KEY
            Address = 10.8.0.2/24
            DNS = 1.1.1.1

            [Peer]
            PublicKey = $SERVER_PUBLIC_KEY
            Endpoint = vpn.example.com:51820
            AllowedIPs = 0.0.0.0/0
            PersistentKeepalive = 25
        """.trimIndent()
    }
}
