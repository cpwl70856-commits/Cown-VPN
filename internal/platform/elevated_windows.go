//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

// IsElevated reports whether the process has an elevated Administrator token.
func IsElevated() bool {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}
