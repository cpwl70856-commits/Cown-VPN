package tunnel

import (
	"fmt"
	"net"
	"strings"
)

// RouteMode controls how AllowedIPs / default routes are applied.
type RouteMode string

const (
	// RouteModeSplit only installs non-default AllowedIPs (分流).
	RouteModeSplit RouteMode = "split"
	// RouteModeGlobal installs full-tunnel half-routes 0.0.0.0/1 + 128.0.0.0/1 (全局).
	RouteModeGlobal RouteMode = "global"
	// RouteModeNone installs no OS routes (用于「测试配置」握手验证).
	RouteModeNone RouteMode = "none"
)

// AppliedRoute records a route that must be removed on disconnect.
// Destination is a CIDR on Unix; on Windows it is "dest|mask" for netsh/route delete.
type AppliedRoute struct {
	Destination string
	Iface       string
	Kind        string // "net" | "default-half"
}

func hasFullTunnel(allowed []string) bool {
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "0.0.0.0/0" || a == "::/0" {
			return true
		}
	}
	return false
}

// expandAllowedForMode returns OS routes to install.
// Global mode always uses half-default routes so we can remove them cleanly
// without touching the host's original default gateway.
func expandAllowedForMode(allowed []string, mode RouteMode) (routes []string, full bool) {
	if mode == RouteModeNone {
		return nil, false
	}
	full = hasFullTunnel(allowed) || mode == RouteModeGlobal
	if mode == RouteModeGlobal {
		return []string{"0.0.0.0/1", "128.0.0.0/1"}, true
	}
	out := make([]string, 0, len(allowed))
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == "0.0.0.0/0" || a == "::/0" {
			// Split mode: never install a true default; that would break host networking
			// if cleanup failed. Users who need full tunnel should use global mode.
			continue
		}
		out = append(out, a)
	}
	return out, false
}

func skipLocalNetwork(dest string, localCIDR string) bool {
	localIP, _, err := net.ParseCIDR(localCIDR)
	if err != nil || localIP == nil {
		return false
	}
	_, n, err := net.ParseCIDR(dest)
	if err != nil {
		return false
	}
	return n.Contains(localIP)
}

// GlobalRouteCommands returns shell/cmd snippets the user can run manually for full tunnel.
func GlobalRouteCommands(iface string) []string {
	return globalRouteCommands(iface)
}

// FormatRouteError returns a Chinese tip when route setup fails due to privileges.
func FormatRouteError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("路由设置失败（可能缺少管理员权限）：%v\nLinux: ip route；macOS: route；Windows: netsh/route。可复制一键命令手动执行。断开时会尝试清理本会话已添加的路由。", err)
}

// DescribeRoutes returns a short Chinese summary of tracked session routes.
func DescribeRoutes(routes []AppliedRoute) string {
	if len(routes) == 0 {
		return "当前无本会话安装的路由"
	}
	var parts []string
	for _, r := range routes {
		label := r.Destination
		if r.Kind == "default-half" {
			label = r.Destination + "（全局半默认）"
		}
		parts = append(parts, fmt.Sprintf("%s → %s", label, r.Iface))
	}
	return strings.Join(parts, "; ")
}
