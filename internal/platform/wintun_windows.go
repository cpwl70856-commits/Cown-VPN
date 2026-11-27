//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DetectWintun looks for wintun.dll next to the executable or in arch subdirs.
func DetectWintun() (ok bool, detail string) {
	var candidates []string
	arch := runtime.GOARCH
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "wintun.dll"),
			filepath.Join(dir, arch, "wintun.dll"),
			filepath.Join(dir, "amd64", "wintun.dll"),
			filepath.Join(dir, "arm64", "wintun.dll"),
			filepath.Join(dir, "x86", "wintun.dll"),
			filepath.Join(dir, "x64", "wintun.dll"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "wintun.dll"),
			filepath.Join(wd, arch, "wintun.dll"),
		)
	}
	// Official WireGuard install location (best-effort).
	candidates = append(candidates,
		`C:\Program Files\WireGuard\wintun.dll`,
		`C:\Program Files\WireGuard\`+arch+`\wintun.dll`,
	)

	seen := map[string]struct{}{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return true, fmt.Sprintf("已找到 %s（架构 %s）。若仍创建 TUN 失败，请确认以管理员运行，且 DLL 架构与本程序一致（当前 GOARCH=%s）。", c, arch, arch)
		}
	}
	return false, fmt.Sprintf(
		"未找到 wintun.dll（当前架构 %s）。\n"+
			"排查步骤：\n"+
			"1. 打开 https://www.wintun.net 下载发布包\n"+
			"2. 解压后将对应架构的 wintun.dll 放到本程序 .exe 同目录\n"+
			"3. 或以管理员安装官方 WireGuard for Windows\n"+
			"4. 重新「以管理员身份运行」本程序，再点「测试 TUN」", arch)
}
