package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProfileIndex tracks named WireGuard profiles on disk.
type ProfileIndex struct {
	DefaultID string         `json:"default_id"`
	Profiles  []ProfileMeta  `json:"profiles"`
}

// ProfileMeta is lightweight metadata for the profile list UI.
type ProfileMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	File      string    `json:"file"` // relative under profiles/
	UpdatedAt time.Time `json:"updated_at"`
}

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-\p{Han}. ]+`)

// ProfilesDir is ~/.config/mywgclient/profiles
func ProfilesDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles"), nil
}

func profileIndexPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles.json"), nil
}

// LoadProfileIndex reads profiles.json (or empty index).
func LoadProfileIndex() (*ProfileIndex, error) {
	path, err := profileIndexPath()
	if err != nil {
		return &ProfileIndex{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProfileIndex{Profiles: nil}, nil
		}
		return &ProfileIndex{}, err
	}
	idx := &ProfileIndex{}
	if err := json.Unmarshal(data, idx); err != nil {
		return &ProfileIndex{}, err
	}
	if idx.Profiles == nil {
		idx.Profiles = []ProfileMeta{}
	}
	return idx, nil
}

func saveProfileIndex(idx *ProfileIndex) error {
	path, err := profileIndexPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func sanitizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "profile"
	}
	name = safeNameRe.ReplaceAllString(name, "_")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func newProfileID(name string) string {
	base := sanitizeProfileName(name)
	base = strings.ReplaceAll(base, " ", "_")
	return fmt.Sprintf("%s_%d", base, time.Now().UnixNano()%1_000_000_000)
}

// ListProfiles returns profile metadata (default first).
func ListProfiles() ([]ProfileMeta, string, error) {
	idx, err := LoadProfileIndex()
	if err != nil {
		return nil, "", err
	}
	return append([]ProfileMeta(nil), idx.Profiles...), idx.DefaultID, nil
}

// SaveNamedProfile writes cfg under profiles/ and updates the index.
// If id is empty a new profile is created. Returns the profile id.
func SaveNamedProfile(id, name string, cfg *Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	name = sanitizeProfileName(name)
	if name == "" {
		name = "未命名"
	}
	dir, err := ProfilesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	idx, err := LoadProfileIndex()
	if err != nil {
		idx = &ProfileIndex{}
	}
	if id == "" {
		id = newProfileID(name)
		file := id + ".conf"
		path := filepath.Join(dir, file)
		cfg.Name = name
		if err := SaveFile(path, cfg); err != nil {
			return "", err
		}
		idx.Profiles = append(idx.Profiles, ProfileMeta{
			ID: id, Name: name, File: file, UpdatedAt: time.Now(),
		})
		if idx.DefaultID == "" {
			idx.DefaultID = id
		}
		return id, saveProfileIndex(idx)
	}
	found := false
	for i := range idx.Profiles {
		if idx.Profiles[i].ID == id {
			idx.Profiles[i].Name = name
			idx.Profiles[i].UpdatedAt = time.Now()
			if idx.Profiles[i].File == "" {
				idx.Profiles[i].File = id + ".conf"
			}
			path := filepath.Join(dir, idx.Profiles[i].File)
			cfg.Name = name
			if err := SaveFile(path, cfg); err != nil {
				return "", err
			}
			found = true
			break
		}
	}
	if !found {
		file := id + ".conf"
		path := filepath.Join(dir, file)
		cfg.Name = name
		if err := SaveFile(path, cfg); err != nil {
			return "", err
		}
		idx.Profiles = append(idx.Profiles, ProfileMeta{
			ID: id, Name: name, File: file, UpdatedAt: time.Now(),
		})
	}
	return id, saveProfileIndex(idx)
}

// LoadNamedProfile loads a profile by id.
func LoadNamedProfile(id string) (*Config, *ProfileMeta, error) {
	idx, err := LoadProfileIndex()
	if err != nil {
		return nil, nil, err
	}
	dir, err := ProfilesDir()
	if err != nil {
		return nil, nil, err
	}
	for i := range idx.Profiles {
		p := &idx.Profiles[i]
		if p.ID != id {
			continue
		}
		path := filepath.Join(dir, p.File)
		cfg, err := LoadFile(path)
		if err != nil {
			return nil, nil, err
		}
		cfg.Name = p.Name
		return cfg, p, nil
	}
	return nil, nil, fmt.Errorf("未找到配置档案: %s", id)
}

// RenameProfile changes the display name.
func RenameProfile(id, newName string) error {
	newName = sanitizeProfileName(newName)
	if newName == "" {
		return fmt.Errorf("名称不能为空")
	}
	idx, err := LoadProfileIndex()
	if err != nil {
		return err
	}
	for i := range idx.Profiles {
		if idx.Profiles[i].ID == id {
			idx.Profiles[i].Name = newName
			idx.Profiles[i].UpdatedAt = time.Now()
			return saveProfileIndex(idx)
		}
	}
	return fmt.Errorf("未找到配置档案: %s", id)
}

// DeleteProfile removes a profile file and index entry.
func DeleteProfile(id string) error {
	idx, err := LoadProfileIndex()
	if err != nil {
		return err
	}
	dir, err := ProfilesDir()
	if err != nil {
		return err
	}
	out := make([]ProfileMeta, 0, len(idx.Profiles))
	var removed *ProfileMeta
	for _, p := range idx.Profiles {
		if p.ID == id {
			cp := p
			removed = &cp
			continue
		}
		out = append(out, p)
	}
	if removed == nil {
		return fmt.Errorf("未找到配置档案: %s", id)
	}
	_ = os.Remove(filepath.Join(dir, removed.File))
	idx.Profiles = out
	if idx.DefaultID == id {
		idx.DefaultID = ""
		if len(out) > 0 {
			idx.DefaultID = out[0].ID
		}
	}
	return saveProfileIndex(idx)
}

// SetDefaultProfile marks id as the default profile and mirrors it to profile.conf.
func SetDefaultProfile(id string) error {
	cfg, meta, err := LoadNamedProfile(id)
	if err != nil {
		return err
	}
	idx, err := LoadProfileIndex()
	if err != nil {
		return err
	}
	idx.DefaultID = id
	if err := saveProfileIndex(idx); err != nil {
		return err
	}
	path, err := DefaultProfilePath()
	if err != nil {
		return err
	}
	cfg.Name = meta.Name
	if err := SaveFile(path, cfg); err != nil {
		return err
	}
	s, _ := LoadSettings()
	if s == nil {
		s = DefaultSettings()
	}
	s.DefaultProfileID = id
	s.LastProfilePath = path
	return SaveSettings(s)
}

// ExportProfile writes the profile conf to destPath.
func ExportProfile(id, destPath string) error {
	cfg, _, err := LoadNamedProfile(id)
	if err != nil {
		return err
	}
	return SaveFile(destPath, cfg)
}

// EnsureMigratedProfile moves legacy profile.conf into the multi-profile store once.
func EnsureMigratedProfile() error {
	idx, err := LoadProfileIndex()
	if err != nil {
		return err
	}
	if len(idx.Profiles) > 0 {
		return nil
	}
	path, err := DefaultProfilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	cfg, err := LoadFile(path)
	if err != nil {
		return err
	}
	name := cfg.Name
	if name == "" || name == "imported" || name == "profile" {
		name = "默认配置"
	}
	id, err := SaveNamedProfile("", name, cfg)
	if err != nil {
		return err
	}
	return SetDefaultProfile(id)
}
