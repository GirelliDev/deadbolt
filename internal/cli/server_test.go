package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A restore target or a fresh install has no <server>.ini yet; load must not fail on that.
func TestLoadWithoutIni(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "d.toml")
	os.WriteFile(cfg, []byte("[server]\ndata_dir = \""+filepath.Join(dir, "data")+"\"\nservice = \"nope.service\"\n"), 0o600)
	var errw bytes.Buffer
	e, code := load("test", []string{"-config", cfg}, &errw)
	if e == nil || code != -1 {
		t.Fatalf("load failed without an ini: code=%d err=%q", code, errw.String())
	}
	if len(e.ini) != 0 {
		t.Fatalf("ini should be empty: %v", e.ini)
	}
}

func TestReorderFlagsAnywhere(t *testing.T) {
	got := reorder([]string{"set", "Key", "Val", "-force", "-config", "/x.toml", "-mod", "A,B"})
	want := []string{"-force", "-config", "/x.toml", "-mod", "A,B", "set", "Key", "Val"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}
