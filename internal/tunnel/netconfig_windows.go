//go:build windows

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
	mask := net.IP(ipNet.Mask).String()
	out, err := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", iface), "static", ip.String(), mask).CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh: %w (%s)", err, strings.TrimSpace(string(out)))
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
		ip, ipNet, err := net.ParseCIDR(a)
		if err != nil {
			continue
		}
		mask := net.IP(ipNet.Mask).String()
		dest := ip.Mask(ipNet.Mask).String()
		out, err := exec.Command("netsh", "interface", "ip", "add", "route",
			dest, mask, iface).CombinedOutput()
		if err != nil {
			// Fallback to route.exe
			out2, err2 := exec.Command("route", "add", dest, "mask", mask, "0.0.0.0", "IF", iface).CombinedOutput()
			if err2 != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("route %s: %w (%s / %s)", a, err2,
						strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
				}
				continue
			}
		}
		kind := "net"
		if a == "0.0.0.0/1" || a == "128.0.0.0/1" {
			kind = "default-half"
		}
		// Encode dest|mask for cleanup with netsh/route delete.
		applied = append(applied, AppliedRoute{Destination: dest + "|" + mask, Iface: iface, Kind: kind})
	}
	return applied, firstErr
}

// removeRoutes deletes session routes; returns how many deletions failed.
func removeRoutes(routes []AppliedRoute) (failed int) {
	for i := len(routes) - 1; i >= 0; i-- {
		r := routes[i]
		parts := strings.SplitN(r.Destination, "|", 2)
		if len(parts) != 2 {
			failed++
			continue
		}
		err1 := exec.Command("netsh", "interface", "ip", "delete", "route", parts[0], parts[1], r.Iface).Run()
		err2 := exec.Command("route", "delete", parts[0], "mask", parts[1]).Run()
		if err1 != nil && err2 != nil {
			failed++
		}
	}
	return failed
}

func globalRouteCommands(iface string) []string {
	return []string{
		fmt.Sprintf(`netsh interface ip add route 0.0.0.0 128.0.0.0 "%s"`, iface),
		fmt.Sprintf(`netsh interface ip add route 128.0.0.0 128.0.0.0 "%s"`, iface),
		fmt.Sprintf(`REM 断开后: netsh interface ip delete route 0.0.0.0 128.0.0.0 "%s" & netsh interface ip delete route 128.0.0.0 128.0.0.0 "%s"`, iface, iface),
	}
}

func testCreateTUN(mtu int) (string, error) {
	if mtu <= 0 {
		mtu = 1420
	}
	dev, err := createTUNForTest("wgtest", mtu)
	if err != nil {
		return "", err
	}
	name, _ := dev.Name()
	_ = dev.Close()
	return name, nil
}
