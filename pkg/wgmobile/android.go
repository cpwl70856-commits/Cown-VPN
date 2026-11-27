//go:build android

package wgmobile

import (
	"fmt"

	"github.com/user/mywgclient/internal/config"

	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/tun"
)

// adoptAndroidTUN duplicates tunFd and wraps it as a wireguard-go TUN.
// The caller keeps ownership of tunFd and may close it after StartWithFD returns.
func adoptAndroidTUN(_ *config.Config, tunFd int) (tun.Device, conn.Bind, error) {
	if tunFd < 0 {
		return nil, nil, fmt.Errorf("无效的 TUN fd")
	}
	dup, err := unix.Dup(tunFd)
	if err != nil {
		return nil, nil, fmt.Errorf("dup TUN fd: %w", err)
	}
	unix.CloseOnExec(dup)
	if err := unix.SetNonblock(dup, true); err != nil {
		unix.Close(dup)
		return nil, nil, fmt.Errorf("TUN nonblock: %w", err)
	}
	dev, _, err := tun.CreateUnmonitoredTUNFromFD(dup)
	if err != nil {
		// CreateUnmonitoredTUNFromFD adopts dup once its own SetNonblock has
		// succeeded. The os.File finalizer closes the fd if the device is not
		// returned. Do not Close dup here; that races the finalizer.
		return nil, nil, fmt.Errorf("打开 Android TUN 失败: %w", err)
	}
	return dev, conn.NewStdNetBind(), nil
}
