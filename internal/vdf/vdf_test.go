package vdf

import "testing"

func TestParse(t *testing.T) {
	n, err := Parse("\"AppState\"\n{\n\t\"buildid\"\t\t\"123\"\n\t\"UserConfig\"\n\t{\n\t\t\"BetaKey\" \"unstable\" // c\n\t}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := n.String("AppState", "buildid"); v != "123" {
		t.Fatal(v)
	}
	if v, _ := n.String("AppState", "UserConfig", "BetaKey"); v != "unstable" {
		t.Fatal(v)
	}
	if _, ok := n.String("AppState", "nope"); ok {
		t.Fatal("missing key reported present")
	}
	if _, err := Parse(`"a" { "b" "c"`); err == nil {
		t.Fatal("unterminated block must fail")
	}
}
