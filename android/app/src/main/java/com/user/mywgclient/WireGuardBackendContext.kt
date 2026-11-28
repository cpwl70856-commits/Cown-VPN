package com.user.mywgclient

import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import com.wireguard.android.backend.GoBackend

/**
 * GoBackend starts [GoBackend.VpnService] by class name. Redirect that explicit start to
 * [CownVpnService], which posts the foreground notification and then delegates to GoBackend.
 */
internal class WireGuardBackendContext(base: Context) : ContextWrapper(base.applicationContext) {
    override fun startService(service: Intent): android.content.ComponentName? {
        if (service.component?.className != GoBackend.VpnService::class.java.name) {
            return super.startService(service)
        }
        val redirected = Intent(service).setClass(this, CownVpnService::class.java)
        return baseContext.startForegroundService(redirected)
    }
}
