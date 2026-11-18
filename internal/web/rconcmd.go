package web

import (
	"fmt"
	"regexp"
	"strings"
)

// Everything typed in the browser ends up inside an RCON command line, so each
// field is checked against a strict allowlist before it is quoted. A name or
// message can never contain a quote, backslash or control character.
var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.\-]{0,39}$`)
	itemRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}\.[A-Za-z0-9_]{1,80}$`)
	levels = map[string]bool{"admin": true, "moderator": true, "overseer": true, "gm": true, "observer": true, "none": true}
)

type Params struct {
	User    string `json:"user"`
	Target  string `json:"target"`
	Item    string `json:"item"`
	Count   int    `json:"count"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

// text accepts printable text without characters that could break out of a quoted RCON argument.
func text(field, s string, max int) error {
	if len(s) == 0 || len(s) > max {
		return fmt.Errorf("%s must be 1-%d characters", field, max)
	}
	for _, r := range s {
		if r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains a character that is not allowed", field)
		}
	}
	return nil
}

func name(field, s string) error {
	if !nameRe.MatchString(s) {
		return fmt.Errorf("%s: only letters, digits, space, _ . - (max 40)", field)
	}
	return nil
}

// BuildCommand turns a typed admin action into one RCON command line.
func BuildCommand(action string, p Params) (string, error) {
	switch action {
	case "save":
		return "save", nil
	case "servermsg":
		if err := text("message", strings.TrimSpace(p.Message), 200); err != nil {
			return "", err
		}
		return fmt.Sprintf(`servermsg "%s"`, strings.TrimSpace(p.Message)), nil
	case "kick", "ban":
		if err := name("user", p.User); err != nil {
			return "", err
		}
		cmd := fmt.Sprintf(`%suser "%s"`, action, p.User)
		if p.Reason != "" {
			if err := text("reason", p.Reason, 100); err != nil {
				return "", err
			}
			cmd += fmt.Sprintf(` -r "%s"`, p.Reason)
		}
		return cmd, nil
	case "unban", "godmod":
		if err := name("user", p.User); err != nil {
			return "", err
		}
		if action == "unban" {
			return fmt.Sprintf(`unbanuser "%s"`, p.User), nil
		}
		return fmt.Sprintf(`godmod "%s"`, p.User), nil
	case "additem":
		if err := name("user", p.User); err != nil {
			return "", err
		}
		if !itemRe.MatchString(p.Item) {
			return "", fmt.Errorf("item must look like Base.Axe")
		}
		if p.Count < 1 || p.Count > 500 {
			return "", fmt.Errorf("count must be 1-500")
		}
		return fmt.Sprintf(`additem "%s" "%s" %d`, p.User, p.Item, p.Count), nil
	case "teleport":
		if err := name("user", p.User); err != nil {
			return "", err
		}
		if err := name("target", p.Target); err != nil {
			return "", err
		}
		return fmt.Sprintf(`teleport "%s" "%s"`, p.User, p.Target), nil
	case "setaccesslevel":
		if err := name("user", p.User); err != nil {
			return "", err
		}
		if !levels[p.Level] {
			return "", fmt.Errorf("level must be one of admin, moderator, overseer, gm, observer, none")
		}
		return fmt.Sprintf(`setaccesslevel "%s" "%s"`, p.User, p.Level), nil
	}
	return "", fmt.Errorf("unknown action %q", action)
}

var playersHdr = regexp.MustCompile(`(?m)^Players connected \((\d+)\)`)

// ParsePlayers reads the reply of the `players` command.
func ParsePlayers(reply string) []string {
	if !playersHdr.MatchString(reply) {
		return nil
	}
	var out []string
	for _, l := range strings.Split(reply, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "-") {
			if n := strings.TrimSpace(strings.TrimPrefix(l, "-")); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}
