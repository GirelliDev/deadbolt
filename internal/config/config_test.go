package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndOverride(t *testing.T) {
	wd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	t.Cleanup(func() { os.Chdir(wd) })
	t.Setenv("DEADBOLT_CONFIG", "")
	c, err := Load("")
	if err != nil || c.Server.Name != "servertest" || c.Backup.KeepDaily != 7 {
		t.Fatalf("defaults: %v %+v", err, c)
	}
	p := filepath.Join(t.TempDir(), "d.toml")
	os.WriteFile(p, []byte("[server]\nname = \"omega\"\n"), 0o600)
	c, err = Load(p)
	if err != nil || c.Server.Name != "omega" || c.Server.Service != "pz.service" {
		t.Fatalf("override: %v %+v", err, c)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("explicit missing file must error")
	}
}

func TestReadINI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.ini")
	os.WriteFile(p, []byte("# c\nRCONPort=27015\nMods=a;b;;c\nPublicName=My = Server\n"), 0o600)
	m, err := ReadINI(p)
	if err != nil || m["RCONPort"] != "27015" || m["PublicName"] != "My = Server" {
		t.Fatalf("%v %v", err, m)
	}
	if ListCount(m["Mods"]) != 3 {
		t.Fatal("ListCount")
	}
}
