package com.user.mywgclient

import android.content.Context
import com.wireguard.android.backend.BackendException
import com.wireguard.config.BadConfigException
import com.wireguard.config.Config
import java.io.ByteArrayInputStream

internal const val MAX_CONF_CHARS = 256 * 1024

internal data class ConfigSummary(
    val config: Config,
    val address: String,
    val endpoint: String,
    val dns: String,
    val routes: String,
)

internal sealed class ConfigFailure {
    data object Empty : ConfigFailure()
    data object TooLarge : ConfigFailure()
    data object NoAddress : ConfigFailure()
    data object NoRoute : ConfigFailure()
    data class Invalid(val detail: String) : ConfigFailure()
}

internal sealed class ConfigInspection {
    data class Ready(val summary: ConfigSummary) : ConfigInspection()
    data class Rejected(val failure: ConfigFailure) : ConfigInspection()
}

internal sealed class StartFailure {
    data object MissingConfig : StartFailure()
    data object VpnNotAuthorized : StartFailure()
    data class Dns(val host: String) : StartFailure()
    data class Other(val detail: String) : StartFailure()
}

/** Parses wg-quick text with the official tunnel library and checks it can be started. */
internal fun inspectConfig(text: String): ConfigInspection {
    val trimmed = text.trim()
    if (trimmed.isEmpty()) return ConfigInspection.Rejected(ConfigFailure.Empty)
    if (trimmed.length > MAX_CONF_CHARS) return ConfigInspection.Rejected(ConfigFailure.TooLarge)
    val config = try {
        ByteArrayInputStream(trimmed.toByteArray(Charsets.UTF_8)).use { input -> Config.parse(input) }
    } catch (error: BadConfigException) {
        return ConfigInspection.Rejected(ConfigFailure.Invalid(describeBadConfig(error)))
    } catch (error: Exception) {
        val detail = error.message?.takeIf { it.isNotBlank() } ?: error.javaClass.simpleName
        return ConfigInspection.Rejected(ConfigFailure.Invalid(detail))
    }
    val addresses = config.getInterface().addresses
    if (addresses.isEmpty()) return ConfigInspection.Rejected(ConfigFailure.NoAddress)
    val routes = config.peers.flatMap { peer -> peer.allowedIps }
    if (routes.isEmpty()) return ConfigInspection.Rejected(ConfigFailure.NoRoute)
    val endpoint = config.peers.firstNotNullOfOrNull { peer ->
        peer.endpoint.orElse(null)?.toString()
    }.orEmpty()
    val dns = config.getInterface().dnsServers.joinToString(", ") { server ->
        server.hostAddress.orEmpty()
    }
    return ConfigInspection.Ready(
        ConfigSummary(
            config = config,
            address = addresses.joinToString(", ") { it.toString() },
            endpoint = endpoint,
            dns = dns,
            routes = routes.joinToString(", ") { it.toString() },
        ),
    )
}

internal fun describeBadConfig(error: BadConfigException): String {
    val why = when (error.reason) {
        BadConfigException.Reason.INVALID_KEY -> "密钥无效"
        BadConfigException.Reason.INVALID_NUMBER -> "数字无效"
        BadConfigException.Reason.INVALID_VALUE -> "取值无效"
        BadConfigException.Reason.MISSING_ATTRIBUTE -> "缺少字段"
        BadConfigException.Reason.MISSING_SECTION -> "缺少段落"
        BadConfigException.Reason.SYNTAX_ERROR -> "语法错误"
        BadConfigException.Reason.UNKNOWN_ATTRIBUTE -> "未知字段"
        BadConfigException.Reason.UNKNOWN_SECTION -> "未知段落"
    }
    val where = error.location.getName().ifBlank { error.section.getName() }
    val extra = error.text?.toString()?.trim().orEmpty()
    return if (extra.isEmpty()) "$why（$where）" else "$why（$where）：$extra"
}

internal fun classifyStartFailure(error: Throwable): StartFailure {
    if (error is BackendException) {
        return when (error.reason) {
            BackendException.Reason.TUNNEL_MISSING_CONFIG -> StartFailure.MissingConfig
            BackendException.Reason.VPN_NOT_AUTHORIZED -> StartFailure.VpnNotAuthorized
            BackendException.Reason.DNS_RESOLUTION_FAILURE ->
                StartFailure.Dns(error.getFormat().firstOrNull()?.toString().orEmpty())
            BackendException.Reason.UNABLE_TO_START_VPN ->
                StartFailure.Other(backendDetail("无法启动 VPN 服务", error))
            BackendException.Reason.TUN_CREATION_ERROR ->
                StartFailure.Other(backendDetail("无法创建 VPN 接口", error))
            BackendException.Reason.GO_ACTIVATION_ERROR_CODE ->
                StartFailure.Other(backendDetail("隧道启动失败", error))
            BackendException.Reason.WG_QUICK_CONFIG_ERROR_CODE ->
                StartFailure.Other(backendDetail("配置无法应用", error))
            BackendException.Reason.UNKNOWN_KERNEL_MODULE_NAME ->
                StartFailure.Other(backendDetail("未知内核模块", error))
        }
    }
    val detail = error.message?.takeIf { it.isNotBlank() } ?: error.javaClass.simpleName
    return StartFailure.Other(detail)
}

internal fun configFailureText(context: Context, failure: ConfigFailure): String = when (failure) {
    ConfigFailure.Empty -> context.getString(R.string.err_empty)
    ConfigFailure.TooLarge -> context.getString(R.string.import_too_large)
    ConfigFailure.NoAddress -> context.getString(R.string.err_address)
    ConfigFailure.NoRoute -> context.getString(R.string.err_no_route)
    is ConfigFailure.Invalid -> context.getString(R.string.err_invalid, failure.detail)
}

internal fun startFailureText(context: Context, error: Throwable): String = when (val kind = classifyStartFailure(error)) {
    StartFailure.MissingConfig -> context.getString(R.string.err_empty)
    StartFailure.VpnNotAuthorized -> context.getString(R.string.vpn_denied)
    is StartFailure.Dns ->
        if (kind.host.isBlank()) {
            context.getString(R.string.err_dns_unknown)
        } else {
            context.getString(R.string.err_dns, kind.host)
        }
    is StartFailure.Other -> context.getString(R.string.connect_failed, kind.detail)
}

internal fun formatTunnelStats(rx: Long, tx: Long, latestHandshakeEpochMillis: Long): String {
    val handshake = if (latestHandshakeEpochMillis > 0L) "已握手" else "尚无握手"
    return "connected  ↓$rx  ↑$tx  $handshake"
}

private fun backendDetail(reason: String, error: BackendException): String {
    val extra = error.getFormat().joinToString(", ") { it.toString() }.trim()
    return if (extra.isEmpty()) reason else "$reason（$extra）"
}
