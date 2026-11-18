package svc

import "testing"

func TestState(t *testing.T) {
	u := Unit{Name: "x.service", Run: func(string, ...string) (string, error) { return "inactive", nil }}
	if u.State() != "inactive" {
		t.Fatal(u.State())
	}
	u.Run = func(string, ...string) (string, error) { return "", nil }
	if u.State() != "unknown" {
		t.Fatal(u.State())
	}
}
