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
	d := func(day, hour int, reason string) Info {
		return Info{Name: reason, Reason: reason, Time: time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC)}
	}
	list := []Info{
		d(18, 9, "auto"), d(18, 3, "auto"), // same day: only the newest stays
		d(17, 3, "auto"), d(16, 3, "auto"),
		d(1, 3, "auto"),   // old, outside 3 days, but week 44 slot? keepWeekly=1 -> only current week
		d(2, 3, "manual"), // manual is never pruned
	}
	del := Prune(list, 3, 1)
	got := map[string]int{}
	for _, b := range del {
		got[b.Time.Format("02-15")]++
	}
	if len(del) != 2 || got["18-03"] != 1 || got["01-03"] != 1 {
		t.Fatalf("deleted: %+v", del)
	}
}
