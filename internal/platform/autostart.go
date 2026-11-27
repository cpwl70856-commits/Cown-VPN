package platform

import "runtime"

// AutostartResult describes the outcome of enabling/disabling login autostart.
type AutostartResult struct {
	OK      bool
	Enabled bool
	Path    string // desktop file / plist / registry value when applicable
	TipZH   string
}

// SetAutostart enables or disables best-effort desktop autostart.
func SetAutostart(enable bool) AutostartResult {
	return setAutostart(enable)
}

// AutostartStatus reports whether autostart appears enabled.
func AutostartStatus() AutostartResult {
	return autostartStatus()
}

// AutostartSupported is true when a platform helper exists.
func AutostartSupported() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		return true
	default:
		return false
	}
}
