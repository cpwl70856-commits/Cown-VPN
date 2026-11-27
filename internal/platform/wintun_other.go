//go:build !windows

package platform

// DetectWintun is only meaningful on Windows.
func DetectWintun() (ok bool, detail string) {
	return true, "非 Windows 平台无需 Wintun"
}
