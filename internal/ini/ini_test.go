package ini

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetKeepsMixedLineEndingsAndComments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.ini")
	os.WriteFile(p, []byte("# note\r\nA=1\r\nB=2\nMods=x;y\r\nLAST=9"), 0o640)
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "10")
	f.Set("B", "20")
	f.Set("New", "n")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "# note\r\nA=10\r\nB=20\nMods=x;y\r\nLAST=9\r\nNew=n\r\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if bak, _ := os.ReadFile(p + ".deadbolt-bak"); string(bak) != "# note\r\nA=1\r\nB=2\nMods=x;y\r\nLAST=9" {
		t.Fatalf("backup %q", bak)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", st.Mode())
	}
	if v, ok := f.Get("Mods"); !ok || v != "x;y" {
		t.Fatalf("%q %v", v, ok)
	}
}

func TestSetRejectsInjection(t *testing.T) {
	f := &File{eol: "\n"}
	if f.Set("A", "1\nB=2") == nil || f.Set("A=B", "1") == nil {
		t.Fatal("newline/equals must be rejected")
	}
}
