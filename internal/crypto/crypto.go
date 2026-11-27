// Package crypto provides WireGuard Curve25519 key helpers.
//
// The tunnel data plane is implemented by wireguard-go (Noise_IK + ChaCha20-Poly1305).
// This package generates / encodes keys in WireGuard base64 and UAPI hex forms.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/curve25519"
)

const KeySize = 32

// KeyPair is a WireGuard Curve25519 identity key pair.
type KeyPair struct {
	Private [KeySize]byte
	Public  [KeySize]byte
}

// GenerateKeyPair creates a clamped Curve25519 private key and its public key.
func GenerateKeyPair() (*KeyPair, error) {
	var priv [KeySize]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	clamp(&priv)
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("x25519: %w", err)
	}
	kp := &KeyPair{Private: priv}
	copy(kp.Public[:], pub)
	return kp, nil
}

// GeneratePSK returns a random 32-byte WireGuard pre-shared key.
func GeneratePSK() ([KeySize]byte, error) {
	var psk [KeySize]byte
	_, err := rand.Read(psk[:])
	return psk, err
}

// PublicFromPrivate derives the public key for a private key.
func PublicFromPrivate(priv [KeySize]byte) ([KeySize]byte, error) {
	clamp(&priv)
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return [KeySize]byte{}, err
	}
	var out [KeySize]byte
	copy(out[:], pub)
	return out, nil
}

func clamp(k *[KeySize]byte) {
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
}

// ParseKey accepts standard WireGuard base64 (44 chars) or 64-char hex.
func ParseKey(s string) ([KeySize]byte, error) {
	s = strings.TrimSpace(s)
	var out [KeySize]byte
	switch {
	case len(s) == 64:
		b, err := hex.DecodeString(s)
		if err != nil {
			return out, fmt.Errorf("hex key: %w", err)
		}
		if len(b) != KeySize {
			return out, fmt.Errorf("hex key: want %d bytes", KeySize)
		}
		copy(out[:], b)
		return out, nil
	case len(s) == 43 || len(s) == 44:
		b, err := base64.StdEncoding.DecodeString(padBase64(s))
		if err != nil {
			return out, fmt.Errorf("base64 key: %w", err)
		}
		if len(b) != KeySize {
			return out, fmt.Errorf("base64 key: want %d bytes, got %d", KeySize, len(b))
		}
		copy(out[:], b)
		return out, nil
	default:
		return out, fmt.Errorf("key must be base64 (43–44 chars) or hex (64 chars), got len=%d", len(s))
	}
}

func padBase64(s string) string {
	switch len(s) % 4 {
	case 2:
		return s + "=="
	case 3:
		return s + "="
	default:
		return s
	}
}

// KeyToBase64 encodes a key in WireGuard's usual base64 form.
func KeyToBase64(k [KeySize]byte) string {
	return base64.StdEncoding.EncodeToString(k[:])
}

// KeyToHex encodes a key for WireGuard UAPI (IpcSet).
func KeyToHex(k [KeySize]byte) string {
	return hex.EncodeToString(k[:])
}
