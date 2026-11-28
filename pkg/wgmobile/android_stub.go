//go:build !android

package wgmobile

import (
	"fmt"
	"runtime"

	"github.com/user/mywgclient/internal/config"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/tun"
)

func adoptAndroidTUN(_ *config.Config, _ int) (tun.Device, conn.Bind, error) {
	return nil, nil, fmt.Errorf("StartWithFD 仅支持 Android（当前 %s）", runtime.GOOS)
}
