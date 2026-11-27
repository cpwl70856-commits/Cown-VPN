//go:build unix

package platform

import "os"

// IsElevated reports whether the process is running as root (uid 0).
func IsElevated() bool {
	return os.Geteuid() == 0
}
