package mods

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLists(t *testing.T) {
	if got := Add("a;b", "b", "c", ""); got != "a;b;c" {
		t.Fatal(got)
	}
	if got := Add("", "x"); got != "x" {
		t.Fatal(got)
	}
	if got := Remove("a;TacHold AR;c", "TacHold AR"); got != "a;c" {
		t.Fatal(got)
	}
	if !Has("a;b", "b") || Has("a;b", "c") {
		t.Fatal("Has")
	}
}

func TestModIDs(t *testing.T) {
	d := t.TempDir()
	w := func(rel, c string) {
		p := filepath.Join(d, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(c), 0o644)
	}
	w("mods/A/42/mod.info", "name=A\r\nid=ModA\r\nversionMin=42\r\n")
	w("mods/A/common/mod.info", "id=ModA\n") // duplicate across version folders
	w("mods/B/mod.info", "\xef\xbb\xbfname=B\nID = ModB \n")
	w("mods/C/notes.txt", "id=NotAMod\n")
	got, err := ModIDs(d)
	if err != nil || !reflect.DeepEqual(got, []string{"ModA", "ModB"}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ModIDs(filepath.Join(d, "missing")); err == nil {
		t.Fatal("missing item dir must error")
	}
}

func TestValidWorkshopID(t *testing.T) {
	if !ValidWorkshopID("3786120850") || ValidWorkshopID("12; rm -rf") || ValidWorkshopID("") || ValidWorkshopID("12") {
		t.Fatal("ValidWorkshopID")
	}
}
