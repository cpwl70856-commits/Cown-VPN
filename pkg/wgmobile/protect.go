package wgmobile

import (
	"fmt"

	"golang.zx2c4.com/wireguard/conn"
)

// protectingBind calls Protector when wireguard-go opens its UDP sockets.
type protectingBind struct {
	conn.Bind
}

func (b *protectingBind) Open(uport uint16) ([]conn.ReceiveFunc, uint16, error) {
	if b == nil || b.Bind == nil {
		return nil, 0, fmt.Errorf("bind is nil")
	}
	fns, port, err := b.Bind.Open(uport)
	if err != nil {
		return nil, 0, err
	}
	protectOpenSockets(b.Bind)
	return fns, port, nil
}

func protectOpenSockets(b conn.Bind) {
	peek, ok := b.(conn.PeekLookAtSocketFd)
	if !ok || peek == nil {
		return
	}
	mu.Lock()
	p := protector
	mu.Unlock()
	if p == nil {
		return
	}
	if fd := callPeek(peek.PeekLookAtSocketFd4); fd >= 0 {
		p.Protect(fd)
	}
	if fd := callPeek(peek.PeekLookAtSocketFd6); fd >= 0 {
		p.Protect(fd)
	}
}

// SocketV4 returns the IPv4 UDP socket fd, or -1 when it is not open.
// Java: long socketV4(). Pass it to VpnService.protect after StartWithFD.
func SocketV4() int { return peekActive(true) }

// SocketV6 returns the IPv6 UDP socket fd, or -1 when it is not open.
func SocketV6() int { return peekActive(false) }

func peekActive(v4 bool) int {
	mu.Lock()
	b := activeBind
	mu.Unlock()
	peek, ok := b.(conn.PeekLookAtSocketFd)
	if !ok || peek == nil {
		return -1
	}
	if v4 {
		return callPeek(peek.PeekLookAtSocketFd4)
	}
	return callPeek(peek.PeekLookAtSocketFd6)
}

// callPeek hides a nil socket. StdNetBind.PeekLookAtSocketFd6 panics when the
// IPv6 listener was not created (EAFNOSUPPORT); Android still needs IPv4.
func callPeek(fn func() (int, error)) (fd int) {
	defer func() {
		if recover() != nil {
			fd = -1
		}
	}()
	got, err := fn()
	if err != nil || got < 0 {
		return -1
	}
	return got
}
