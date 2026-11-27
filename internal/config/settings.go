package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Settings persists UI preferences and last successful connection metadata.
type Settings struct {
	WizardCompleted  bool      `json:"wizard_completed"`
	OnboardingDone   bool      `json:"onboarding_done"`
	RouteMode        string    `json:"route_mode"` // "split" | "global"
	LastSuccessAt    time.Time `json:"last_success_at,omitempty"`
	MinimizeToTray   bool      `json:"minimize_to_tray"`
	AutoReconnect    bool      `json:"auto_reconnect"`
	LastProfilePath  string    `json:"last_profile_path,omitempty"`
	DefaultProfileID string    `json:"default_profile_id,omitempty"`
	Autostart        bool      `json:"autostart"`
	LogLevel         string    `json:"log_level"` // "all" | "info" | "warn" | "error"
	FollowSystemTheme bool     `json:"follow_system_theme"`
}

// DefaultSettings returns sensible first-run defaults.
func DefaultSettings() *Settings {
	return &Settings{
		WizardCompleted:   false,
		OnboardingDone:    false,
		RouteMode:         "split",
		MinimizeToTray:    true,
		AutoReconnect:     false,
		Autostart:         false,
		LogLevel:          "all",
		FollowSystemTheme: true,
	}
}

// SettingsPath is ~/.config/mywgclient/settings.json
func SettingsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// LoadSettings reads settings.json or returns defaults.
func LoadSettings() (*Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return DefaultSettings(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return DefaultSettings(), err
	}
	s := DefaultSettings()
	if err := json.Unmarshal(data, s); err != nil {
		return DefaultSettings(), err
	}
	if s.RouteMode == "" {
		s.RouteMode = "split"
	}
	if s.LogLevel == "" {
		s.LogLevel = "all"
	}
	return s, nil
}

// SaveSettings writes settings.json.
func SaveSettings(s *Settings) error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// RememberSuccess saves the profile and marks last successful connection.
func RememberSuccess(cfg *Config, routeMode string) error {
	path, err := DefaultProfilePath()
	if err != nil {
		return err
	}
	if err := SaveFile(path, cfg); err != nil {
		return err
	}
	// Also refresh the named default profile when one exists.
	s, _ := LoadSettings()
	if s == nil {
		s = DefaultSettings()
	}
	if s.DefaultProfileID != "" {
		name := cfg.Name
		if name == "" {
			name = "默认配置"
		}
		_, _ = SaveNamedProfile(s.DefaultProfileID, name, cfg)
	}
	s.WizardCompleted = true
	s.LastSuccessAt = time.Now()
	s.LastProfilePath = path
	if routeMode != "" {
		s.RouteMode = routeMode
	}
	return SaveSettings(s)
}
