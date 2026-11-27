package tunnel

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	cleanupMu     sync.Mutex
	cleanupTun    *Tunnel
	cleanupOnce   sync.Once
	signalInstalled bool
)

// RegisterForCleanup marks t as the active tunnel whose routes/device must be
// torn down on SIGINT/SIGTERM or process exit helpers. Pass nil to clear.
func RegisterForCleanup(t *Tunnel) {
	cleanupMu.Lock()
	cleanupTun = t
	cleanupMu.Unlock()
	ensureSignalCleanup()
}

// ensureSignalCleanup installs a once-only signal handler that disconnects
// the registered tunnel so global routes do not survive a crash-exit.
func ensureSignalCleanup() {
	cleanupOnce.Do(func() {
		signalInstalled = true
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-ch
			slog.Info("signal received, cleaning tunnel", "signal", sig.String())
			ForceCleanup("signal:" + sig.String())
			// Re-raise default so a second signal can kill if cleanup hangs.
			signal.Stop(ch)
			os.Exit(0)
		}()
	})
}

// ForceCleanup disconnects the registered tunnel (best-effort route removal).
func ForceCleanup(reason string) {
	cleanupMu.Lock()
	t := cleanupTun
	cleanupMu.Unlock()
	if t == nil {
		return
	}
	slog.Info("force cleanup tunnel", "reason", reason)
	_ = t.Disconnect()
}

// GuardPanic runs fn and, on panic, cleans routes before re-panicking.
func GuardPanic(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			ForceCleanup("panic")
			panic(r)
		}
	}()
	fn()
}
