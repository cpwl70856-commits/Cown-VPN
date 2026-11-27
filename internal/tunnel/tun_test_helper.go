package tunnel

import "golang.zx2c4.com/wireguard/tun"

func createTUNForTest(name string, mtu int) (tun.Device, error) {
	return tun.CreateTUN(name, mtu)
}
