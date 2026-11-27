package config

import (
	"path/filepath"
	"testing"
)

func TestNamedProfilesCRUD(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := QuickSetup("1.2.3.4:51820", "WGV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=", "YCV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=")
	id, err := SaveNamedProfile("", "测试档案", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	list, def, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || def != id {
		t.Fatalf("list=%+v def=%s", list, def)
	}
	got, meta, err := LoadNamedProfile(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "测试档案" || got.PrimaryPeer().Endpoint != "1.2.3.4:51820" {
		t.Fatalf("meta=%+v cfg=%+v", meta, got)
	}
	if err := RenameProfile(id, "新名称"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "export.conf")
	if err := ExportProfile(id, out); err != nil {
		t.Fatal(err)
	}
	if err := SetDefaultProfile(id); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProfile(id); err != nil {
		t.Fatal(err)
	}
	list, _, err = ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty, got %+v", list)
	}
}

func TestEnsureMigratedProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := QuickSetup("9.9.9.9:51820", "WGV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=", "YCV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=")
	path, err := DefaultProfilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMigratedProfile(); err != nil {
		t.Fatal(err)
	}
	list, _, err := ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 migrated profile, got %+v", list)
	}
	// Second call is no-op.
	if err := EnsureMigratedProfile(); err != nil {
		t.Fatal(err)
	}
	list2, _, _ := ListProfiles()
	if len(list2) != 1 {
		t.Fatalf("duplicate migrate: %+v", list2)
	}
}
