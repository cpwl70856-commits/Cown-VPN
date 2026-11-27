package com.user.mywgclient

import androidx.lifecycle.MutableLiveData

internal object TunnelState {
    val status = MutableLiveData("disconnected")
    val message = MutableLiveData("")
    val stats = MutableLiveData("")
}
