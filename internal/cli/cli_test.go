package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpForms(t *testing.T) {
	for _, a := range []string{"-help", "--help", "-h", "help"} {
		var out, errw bytes.Buffer
		if code := Run([]string{a}, &out, &errw); code != 0 || !strings.Contains(out.String(), "Usage: dbolt") {
			t.Fatalf("%s: code=%d out=%q", a, code, out.String())
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run([]string{"nope"}, &out, &errw); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
