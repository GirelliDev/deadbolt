package sandbox

import "testing"

const sample = "SandboxVars = {\r\n    VERSION = 6,\r\n    -- Zombies = 99,\r\n    Zombies = 1,\r\n    ZombieLore = {\r\n        Speed = 2,\r\n    },\r\n    Other = {\r\n        Speed = 3,\r\n    },\r\n    Name = \"hi, there\",\r\n}\r\n"

func TestGetSet(t *testing.T) {
	for path, want := range map[string]string{"VERSION": "6", "Zombies": "1", "ZombieLore.Speed": "2", "Other.Speed": "3", "Name": `"hi, there"`} {
		if got, err := Get(sample, path); err != nil || got != want {
			t.Errorf("%s: %q %v", path, got, err)
		}
	}
	out, err := Set(sample, "Other.Speed", "1")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := Get(out, "Other.Speed"); v != "1" {
		t.Fatal(v)
	}
	if v, _ := Get(out, "ZombieLore.Speed"); v != "2" {
		t.Fatal("sibling section must be untouched")
	}
	want := "SandboxVars = {\r\n    VERSION = 6,\r\n    -- Zombies = 99,\r\n    Zombies = 1,\r\n    ZombieLore = {\r\n        Speed = 2,\r\n    },\r\n    Other = {\r\n        Speed = 1,\r\n    },\r\n    Name = \"hi, there\",\r\n}\r\n"
	if out != want {
		t.Fatalf("%q", out)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Get(sample, "Nope"); err == nil {
		t.Fatal("missing key")
	}
	if _, err := Set(sample, "Zombies", "{1}"); err == nil {
		t.Fatal("braces rejected")
	}
	if _, err := Get(sample, "Speed"); err == nil {
		t.Fatal("bare nested key must not match")
	}
}
