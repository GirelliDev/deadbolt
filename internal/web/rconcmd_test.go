package web

import (
	"reflect"
	"testing"
)

func TestBuildCommand(t *testing.T) {
	ok := map[string]struct {
		a string
		p Params
		w string
	}{
		"kick":   {"kick", Params{User: "Bob", Reason: "afk too long"}, `kickuser "Bob" -r "afk too long"`},
		"kick0":  {"kick", Params{User: "Bob"}, `kickuser "Bob"`},
		"ban":    {"ban", Params{User: "Mr X-1", Reason: "cheating"}, `banuser "Mr X-1" -r "cheating"`},
		"unban":  {"unban", Params{User: "Bob"}, `unbanuser "Bob"`},
		"item":   {"additem", Params{User: "Bob", Item: "Base.Axe", Count: 2}, `additem "Bob" "Base.Axe" 2`},
		"msg":    {"servermsg", Params{Message: "Restart in 5 min"}, `servermsg "Restart in 5 min"`},
		"save":   {"save", Params{}, `save`},
		"tp":     {"teleport", Params{User: "Bob", Target: "Ann"}, `teleport "Bob" "Ann"`},
		"access": {"setaccesslevel", Params{User: "Bob", Level: "moderator"}, `setaccesslevel "Bob" "moderator"`},
	}
	for n, c := range ok {
		if got, err := BuildCommand(c.a, c.p); err != nil || got != c.w {
			t.Errorf("%s: %q %v", n, got, err)
		}
	}
}

func TestBuildCommandRejectsInjection(t *testing.T) {
	bad := []struct {
		a string
		p Params
	}{
		{"kick", Params{User: `Bob" -r "x`}},
		{"kick", Params{User: "Bob\nsave"}},
		{"kick", Params{User: "Bob", Reason: `x" ; quit "`}},
		{"kick", Params{User: "Bob", Reason: "line1\nline2"}},
		{"kick", Params{User: ""}},
		{"kick", Params{User: "-rf"}},
		{"additem", Params{User: "Bob", Item: `Base.Axe" 999 "`, Count: 1}},
		{"additem", Params{User: "Bob", Item: "Axe", Count: 1}},
		{"additem", Params{User: "Bob", Item: "Base.Axe", Count: 0}},
		{"additem", Params{User: "Bob", Item: "Base.Axe", Count: 100000}},
		{"servermsg", Params{Message: `hi" ; quit`}},
		{"servermsg", Params{Message: "a\\"}},
		{"servermsg", Params{Message: "   "}},
		{"setaccesslevel", Params{User: "Bob", Level: "root"}},
		{"teleport", Params{User: "Bob", Target: `A"`}},
		{"quit", Params{}},
		{"", Params{}},
	}
	for i, c := range bad {
		if got, err := BuildCommand(c.a, c.p); err == nil {
			t.Errorf("case %d (%s %+v) accepted: %q", i, c.a, c.p, got)
		}
	}
}

func TestParsePlayers(t *testing.T) {
	got := ParsePlayers("Players connected (2): \n-Bob\n-Mr X\n")
	if !reflect.DeepEqual(got, []string{"Bob", "Mr X"}) {
		t.Fatalf("%v", got)
	}
	if ParsePlayers("Players connected (0): \n") != nil || ParsePlayers("garbage") != nil {
		t.Fatal("empty/garbage must give nil")
	}
}
