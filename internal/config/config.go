// Package config loads and saves WireGuard-style configuration for mywgclient.
package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultListenPort = 51820
	DefaultMTU        = 1420
	DefaultKeepalive  = 25
	AppName           = "mywgclient"
)

// Peer is a WireGuard [Peer] section.
type Peer struct {
	PublicKey           string
	Endpoint            string
	AllowedIPs          []string
	PresharedKey        string
	PersistentKeepalive int
}

// Config mirrors a wg-quick style conf with [Interface] and [Peer].
type Config struct {
	// Interface
	PrivateKey string
	Address    string // e.g. 10.8.0.2/24
	DNS        []string
	ListenPort int
	MTU        int
	TUNName    string

	// Peers (desktop client typically has one)
	Peers []Peer

	// Optional display name for saved profiles
	Name string
}

// Validate checks required fields before connect.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("配置为空")
	}
	if strings.TrimSpace(c.PrivateKey) == "" {
		return fmt.Errorf("PrivateKey 为必填项")
	}
	if strings.TrimSpace(c.Address) == "" {
		return fmt.Errorf("Address 为必填项（例如 10.8.0.2/24）")
	}
	if len(c.Peers) == 0 {
		return fmt.Errorf("至少需要一个 [Peer]")
	}
	for i, p := range c.Peers {
		if strings.TrimSpace(p.PublicKey) == "" {
			return fmt.Errorf("peer %d: PublicKey 为必填项", i)
		}
		if len(p.AllowedIPs) == 0 {
			return fmt.Errorf("peer %d: AllowedIPs 为必填项", i)
		}
	}
	if c.MTU <= 0 {
		c.MTU = DefaultMTU
	}
	return nil
}

// PrimaryPeer returns the first peer or nil.
func (c *Config) PrimaryPeer() *Peer {
	if c == nil || len(c.Peers) == 0 {
		return nil
	}
	return &c.Peers[0]
}

// LoadFile reads a standard WireGuard / wg-quick config file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %w", err)
	}
	cfg, err := LoadString(string(data))
	if err != nil {
		return nil, err
	}
	cfg.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return cfg, nil
}

// LoadString parses a WireGuard / wg-quick config from text.
func LoadString(text string) (*Config, error) {
	cfg := &Config{
		MTU:     DefaultMTU,
		TUNName: "wg0",
		Name:    "imported",
	}

	sc := bufio.NewScanner(strings.NewReader(text))
	section := ""
	var curPeer *Peer

	flushPeer := func() {
		if curPeer != nil {
			cfg.Peers = append(cfg.Peers, *curPeer)
			curPeer = nil
		}
	}

	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			switch name {
			case "interface":
				flushPeer()
				section = "interface"
			case "peer":
				flushPeer()
				section = "peer"
				curPeer = &Peer{PersistentKeepalive: 0}
			default:
				return nil, fmt.Errorf("第 %d 行：未知小节 %q", lineNo, name)
			}
			continue
		}

		key, val, ok := splitKV(line)
		if !ok {
			return nil, fmt.Errorf("第 %d 行：应为 Key = Value", lineNo)
		}
		keyLower := strings.ToLower(key)

		switch section {
		case "interface":
			if err := applyInterface(cfg, keyLower, val); err != nil {
				return nil, fmt.Errorf("第 %d 行：%w", lineNo, err)
			}
		case "peer":
			if curPeer == nil {
				return nil, fmt.Errorf("第 %d 行：Peer 字段不在 [Peer] 内", lineNo)
			}
			if err := applyPeer(curPeer, keyLower, val); err != nil {
				return nil, fmt.Errorf("第 %d 行：%w", lineNo, err)
			}
		default:
			return nil, fmt.Errorf("第 %d 行：键值不在任何节内", lineNo)
		}
	}
	flushPeer()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ParseImportText accepts pasted WireGuard conf, vpn://… text, or base64-wrapped conf.
func ParseImportText(raw string) (*Config, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, fmt.Errorf("粘贴内容为空")
	}
	// Strip common URI wrappers used by some QR / share links.
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "vpn://"):
		text = strings.TrimSpace(text[len("vpn://"):])
	case strings.HasPrefix(lower, "wireguard://"):
		text = strings.TrimSpace(text[len("wireguard://"):])
	case strings.HasPrefix(lower, "wg://"):
		text = strings.TrimSpace(text[len("wg://"):])
	}
	if cfg, err := LoadString(text); err == nil {
		return cfg, nil
	}
	// Try URL-safe / std base64 of the conf body.
	decoded, err := decodeMaybeBase64(text)
	if err == nil {
		if cfg, err2 := LoadString(decoded); err2 == nil {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("无法解析：请粘贴完整 WireGuard 配置文本（含 [Interface] / [Peer]），或 vpn:// 后跟配置/Base64")
}

func decodeMaybeBase64(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func splitKV(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func applyInterface(cfg *Config, key, val string) error {
	switch key {
	case "privatekey":
		cfg.PrivateKey = val
	case "address":
		// wg-quick allows comma-separated addresses; take the first IPv4-ish for UI simplicity.
		parts := splitCSV(val)
		if len(parts) > 0 {
			cfg.Address = parts[0]
		}
	case "dns":
		cfg.DNS = splitCSV(val)
	case "listenport":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("ListenPort: %w", err)
		}
		cfg.ListenPort = n
	case "mtu":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("MTU: %w", err)
		}
		cfg.MTU = n
	case "table", "preup", "postup", "predown", "postdown", "saveconfig":
		// Ignored by the desktop client (OS routing handled separately).
	default:
		// Ignore unknown keys for forward compatibility.
	}
	return nil
}

func applyPeer(p *Peer, key, val string) error {
	switch key {
	case "publickey":
		p.PublicKey = val
	case "endpoint":
		p.Endpoint = val
	case "allowedips":
		p.AllowedIPs = splitCSV(val)
	case "presharedkey":
		p.PresharedKey = val
	case "persistentkeepalive":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("PersistentKeepalive: %w", err)
		}
		p.PersistentKeepalive = n
	}
	return nil
}

func splitCSV(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SaveFile writes a standard WireGuard conf.
func SaveFile(path string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", cfg.PrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", cfg.Address)
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&b, "ListenPort = %d\n", cfg.ListenPort)
	}
	if cfg.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", cfg.MTU)
	}
	if len(cfg.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(cfg.DNS, ", "))
	}
	for _, p := range cfg.Peers {
		b.WriteString("\n[Peer]\n")
		fmt.Fprintf(&b, "PublicKey = %s\n", p.PublicKey)
		if p.PresharedKey != "" {
			fmt.Fprintf(&b, "PresharedKey = %s\n", p.PresharedKey)
		}
		if len(p.AllowedIPs) > 0 {
			fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(p.AllowedIPs, ", "))
		}
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "Endpoint = %s\n", p.Endpoint)
		}
		if p.PersistentKeepalive > 0 {
			fmt.Fprintf(&b, "PersistentKeepalive = %d\n", p.PersistentKeepalive)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// DefaultProfilePath returns ~/.config/mywgclient/profile.conf (or OS equivalent).
func DefaultProfilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.conf"), nil
}

// ConfigDir is the per-user config directory for mywgclient.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// Prefer XDG on Unix; on macOS/Windows still use .config for simplicity.
	return filepath.Join(home, ".config", AppName), nil
}

// FromForm builds a Config from UI form fields (single-peer client).
func FromForm(privateKey, address, endpoint, publicKey, allowedIPs, psk string, keepalive, listenPort, mtu int) *Config {
	ips := splitCSV(allowedIPs)
	if keepalive < 0 {
		keepalive = 0
	}
	if mtu <= 0 {
		mtu = DefaultMTU
	}
	return &Config{
		PrivateKey: strings.TrimSpace(privateKey),
		Address:    strings.TrimSpace(address),
		ListenPort: listenPort,
		MTU:        mtu,
		TUNName:    "wg0",
		Name:       "profile",
		Peers: []Peer{{
			PublicKey:           strings.TrimSpace(publicKey),
			Endpoint:            strings.TrimSpace(endpoint),
			AllowedIPs:          ips,
			PresharedKey:        strings.TrimSpace(psk),
			PersistentKeepalive: keepalive,
		}},
	}
}

// QuickSetup builds a client config from the three essential wizard fields.
// Defaults: Address=10.8.0.2/24, AllowedIPs=10.8.0.0/24, Keepalive=25, MTU=1420.
func QuickSetup(endpoint, serverPublicKey, clientPrivateKey string) *Config {
	return FromForm(
		clientPrivateKey,
		"10.8.0.2/24",
		endpoint,
		serverPublicKey,
		"10.8.0.0/24",
		"",
		DefaultKeepalive,
		0,
		DefaultMTU,
	)
}

// VirtualIP returns the IP portion of Address (without prefix length).
func (c *Config) VirtualIP() string {
	if c == nil || c.Address == "" {
		return ""
	}
	ip, _, err := splitHostPrefix(c.Address)
	if err != nil {
		return strings.Split(c.Address, "/")[0]
	}
	return ip
}

func splitHostPrefix(cidr string) (string, int, error) {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("not a CIDR")
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], n, nil
}
