package com.user.mywgclient

import org.json.JSONArray
import org.json.JSONObject

internal data class Cidr(val host: String, val prefix: Int) {
    override fun toString(): String = "$host/$prefix"
}

internal data class TunnelHints(
    val address: Cidr?,
    val mtu: Int,
    val dns: List<String>,
    val routes: List<Cidr>,
    val endpoint: String,
)

internal fun parseCidr(raw: String?): Cidr? {
    val text = raw?.trim().orEmpty()
    val slash = text.lastIndexOf('/')
    if (slash <= 0 || slash == text.lastIndex) return null
    val host = text.substring(0, slash).trim()
    val prefix = text.substring(slash + 1).trim().toIntOrNull() ?: return null
    if (host.isEmpty()) return null
    val max = if (host.contains(':')) 128 else 32
    if (prefix !in 0..max) return null
    if (!host.contains(':') && host.count { it == '.' } != 3) return null
    return Cidr(host, prefix)
}

/** Reads the JSON object returned by wgmobile.Wgmobile.parseConf. */
internal fun tunnelHints(json: String): TunnelHints {
    val obj = JSONObject(json)
    val mtuRaw = obj.optInt("mtu", 1420)
    val mtu = if (mtuRaw in 576..10000) mtuRaw else 1420
    return TunnelHints(
        address = parseCidr(obj.optString("address")),
        mtu = mtu,
        dns = jsonStrings(obj.optJSONArray("dns")),
        routes = jsonStrings(obj.optJSONArray("allowed_ips")).mapNotNull { parseCidr(it) },
        endpoint = obj.optString("endpoint"),
    )
}

internal fun formatStats(json: String): String {
    return try {
        val obj = JSONObject(json)
        val state = obj.optString("state", "disconnected")
        val rx = obj.optLong("rx_bytes")
        val tx = obj.optLong("tx_bytes")
        val handshake = if (obj.optLong("last_handshake_unix") > 0) "已握手" else "尚无握手"
        val err = obj.optString("error")
        val line = "$state  ↓$rx  ↑$tx  $handshake"
        if (err.isNullOrEmpty()) line else "$line  $err"
    } catch (_: Exception) {
        json
    }
}

private fun jsonStrings(array: JSONArray?): List<String> {
    if (array == null) return emptyList()
    val out = ArrayList<String>(array.length())
    for (i in 0 until array.length()) {
        val value = array.optString(i).trim()
        if (value.isNotEmpty()) out.add(value)
    }
    return out
}
