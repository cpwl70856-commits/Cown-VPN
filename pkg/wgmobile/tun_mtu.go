package wgmobile

import "golang.zx2c4.com/wireguard/tun"

// fixedMTU reports the MTU from the WireGuard config. Android's VpnService
// already applied that MTU; asking the kernel again can fail on a VpnService fd.
type fixedMTU struct {
	tun.Device
	mtu int
}

func (t *fixedMTU) MTU() (int, error) {
	if t != nil && t.mtu > 0 {
		return t.mtu, nil
	}
	if t == nil || t.Device == nil {
		return 0, errNoTUN
	}
	return t.Device.MTU()
}

func (t *fixedMTU) BatchSize() int {
	if t == nil || t.Device == nil {
		return 1
	}
	if n := t.Device.BatchSize(); n > 0 {
		return n
	}
	return 1
}
