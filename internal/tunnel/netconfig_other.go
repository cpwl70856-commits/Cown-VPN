//go:build !linux && !darwin && !windows

package tunnel

import "fmt"

func configureAddress(iface, cidr string) error {
	return fmt.Errorf("address configuration not supported on this platform (iface=%s addr=%s)", iface, cidr)
}

func applyRoutes(iface string, dests []string, localCIDR string) ([]AppliedRoute, error) {
	return nil, fmt.Errorf("routing not supported on this platform")
}

func removeRoutes(routes []AppliedRoute) int { return 0 }

func globalRouteCommands(iface string) []string {
	return []string{"# routing not supported on this platform"}
}

func testCreateTUN(mtu int) (string, error) {
	return "", fmt.Errorf("TUN test not supported on this platform")
}
