package backup

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func mk(t *testing.T, root string, files map[string]string) {
	for n, c := range files {
		p := filepath.Join(root, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateListRestore(t *testing.T) {
	data, dest := t.TempDir(), t.TempDir()
	mk(t, data, map[string]string{"Saves/Multiplayer/servertest/map.bin": "world", "Server/servertest.ini": "A=1", "db/servertest.db": "db", "Logs/x.log": "ignored"})
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.Local)
	b, err := Create("servertest", data, dest, "manual", now)
	if err != nil || b.Size == 0 {
		t.Fatalf("%v %+v", err, b)
	}
	l, _ := List(dest, "servertest")
	if len(l) != 1 || l[0].Reason != "manual" || !l[0].Time.Equal(now) {
		t.Fatalf("%+v", l)
	}
	if l2, _ := List(dest, "other"); len(l2) != 0 {
		t.Fatal("must filter by server name")
	}
	// Corrupt live data, restore, check content and that the old copy was kept.
	os.WriteFile(filepath.Join(data, "Server/servertest.ini"), []byte("A=2"), 0o644)
	moved, err := Restore(b.Path, data, now.Add(time.Minute))
	if err != nil || len(moved) != 3 {
		t.Fatalf("%v %v", err, moved)
	}
	if got, _ := os.ReadFile(filepath.Join(data, "Server/servertest.ini")); string(got) != "A=1" {
		t.Fatalf("restored %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(data, "Server.pre-restore-20261118-100100/servertest.ini")); string(got) != "A=2" {
		t.Fatalf("old copy %q", got)
	}
	if _, err := os.Stat(filepath.Join(data, "Logs/x.log")); err != nil {
		t.Fatal("Logs must be untouched")
	}
}

func TestCreateNothing(t *testing.T) {
	if _, err := Create("s", t.TempDir(), t.TempDir(), "manual", time.Now()); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestoreRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil", "/etc/passwd", "Other/x", "Saves/../../evil"} {
		arch := filepath.Join(t.TempDir(), "bad.tar.zst")
		f, _ := os.Create(arch)
		zw, _ := zstd.NewWriter(f)
		tw := tar.NewWriter(zw)
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
		tw.Write([]byte("x"))
		tw.Close()
		zw.Close()
		f.Close()
		data := t.TempDir()
		if _, err := Restore(arch, data, time.Now()); err == nil {
			t.Fatalf("%q must be rejected", name)
		}
	}
}

func TestPrune(t *testing.T) {
	at := func(h int, reason string) Info {
		return Info{Name: reason, Reason: reason, Time: time.Date(2026, 11, 18, h, 0, 0, 0, time.UTC)}
	}
	list := []Info{at(9, "manual"), at(12, "pre-update"), at(3, "manual")}
	del := Prune(list, 1)
	if len(del) != 2 || del[0].Time.Hour() != 9 || del[1].Time.Hour() != 3 {
		t.Fatalf("only the newest (12h) may stay, any reason: %+v", del)
	}
	if len(Prune(list, 0)) != 2 || len(Prune(list, -5)) != 2 {
		t.Fatal("keep < 1 must still keep one")
	}
	if len(Prune(list[:1], 1)) != 0 || len(Prune(nil, 1)) != 0 {
		t.Fatal("the only backup must never be deleted")
	}
	if len(Prune(list, 2)) != 1 {
		t.Fatal("keep=2")
	}
}
