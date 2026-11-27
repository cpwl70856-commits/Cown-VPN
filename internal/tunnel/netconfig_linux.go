//go:build linux

package tunnel

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

func configureAddress(iface, cidr string) error {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse address %q: %w", cidr, err)
	}
	_ = exec.Command("ip", "link", "set", "dev", iface, "up").Run()
	out, err := exec.Command("ip", "addr", "replace", fmt.Sprintf("%s/%d", ip.String(), maskLen(ipNet)), "dev", iface).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip addr: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func applyRoutes(iface string, dests []string, localCIDR string) ([]AppliedRoute, error) {
	var applied []AppliedRoute
	var firstErr error
	for _, a := range dests {
		if skipLocalNetwork(a, localCIDR) {
			continue
		}
		out, err := exec.Command("ip", "route", "replace", a, "dev", iface).CombinedOutput()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("ip route %s: %w (%s)", a, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		kind := "net"
		if a == "0.0.0.0/1" || a == "128.0.0.0/1" {
			kind = "default-half"
		}
		applied = append(applied, AppliedRoute{Destination: a, Iface: iface, Kind: kind})
	}
	return applied, firstErr
}

// removeRoutes deletes session routes; returns how many deletions failed.
// Tries "dev <iface>" first, then a bare delete — critical for restoring host networking
// after global half-default routes (0.0.0.0/1 + 128.0.0.0/1).
func removeRoutes(routes []AppliedRoute) (failed int) {
	for i := len(routes) - 1; i >= 0; i-- {
		r := routes[i]
		err := exec.Command("ip", "route", "del", r.Destination, "dev", r.Iface).Run()
		if err != nil {
			err = exec.Command("ip", "route", "del", r.Destination).Run()
		}
		if err != nil {
			failed++
		}
	}
	return failed
}

func globalRouteCommands(iface string) []string {
	return []string{
		fmt.Sprintf("sudo ip route replace 0.0.0.0/1 dev %s", iface),
		fmt.Sprintf("sudo ip route replace 128.0.0.0/1 dev %s", iface),
		fmt.Sprintf("# 断开后清理: sudo ip route del 0.0.0.0/1 dev %s; sudo ip route del 128.0.0.0/1 dev %s", iface, iface),
	}
}

func maskLen(n *net.IPNet) int {
	ones, _ := n.Mask.Size()
	return ones
}

func testCreateTUN(mtu int) (string, error) {
	if mtu <= 0 {
		mtu = 1420
	}
	dev, err := createTUNForTest("wgtest0", mtu)
	if err != nil {
		return "", err
	}
	name, _ := dev.Name()
	_ = dev.Close()
	return name, nil
}
