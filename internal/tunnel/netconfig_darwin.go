//go:build darwin

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
	peer := peerFromCIDR(ip, ipNet)
	out, err := exec.Command("ifconfig", iface, "inet", ip.String(), peer, "up").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ifconfig: %w (%s)", err, strings.TrimSpace(string(out)))
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
		// Prefer -net with CIDR; fall back quietly on failure.
		out, err := exec.Command("route", "-n", "add", "-net", a, "-interface", iface).CombinedOutput()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("route add %s: %w (%s)", a, err, strings.TrimSpace(string(out)))
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
// Dual attempt keeps host networking recoverable if the utun name changed.
func removeRoutes(routes []AppliedRoute) (failed int) {
	for i := len(routes) - 1; i >= 0; i-- {
		r := routes[i]
		err := exec.Command("route", "-n", "delete", "-net", r.Destination, "-interface", r.Iface).Run()
		if err != nil {
			err = exec.Command("route", "-n", "delete", "-net", r.Destination).Run()
		}
		if err != nil {
			failed++
		}
	}
	return failed
}

func globalRouteCommands(iface string) []string {
	return []string{
		fmt.Sprintf("sudo route -n add -net 0.0.0.0/1 -interface %s", iface),
		fmt.Sprintf("sudo route -n add -net 128.0.0.0/1 -interface %s", iface),
		fmt.Sprintf("# 断开后清理: sudo route -n delete -net 0.0.0.0/1 -interface %s; sudo route -n delete -net 128.0.0.0/1 -interface %s", iface, iface),
	}
}

func peerFromCIDR(ip net.IP, n *net.IPNet) string {
	ones, bits := n.Mask.Size()
	if ones >= bits-1 {
		return ip.String()
	}
	peer := make(net.IP, len(ip.To4()))
	copy(peer, ip.To4())
	if peer == nil {
		return ip.String()
	}
	peer[3] ^= 1
	if peer.Equal(ip) {
		peer[3] ^= 2
	}
	return peer.String()
}

func testCreateTUN(mtu int) (string, error) {
	if mtu <= 0 {
		mtu = 1420
	}
	dev, err := createTUNForTest("utun", mtu)
	if err != nil {
		return "", err
	}
	name, _ := dev.Name()
	_ = dev.Close()
	return name, nil
}
