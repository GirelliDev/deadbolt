// Package sandbox reads and edits values in <server>_SandboxVars.lua without
// disturbing its comments: it only rewrites the "Key = value," line it was asked for.
package sandbox

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	openRe  = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\{\s*$`)
	keyRe   = regexp.MustCompile(`^(\s*)([A-Za-z_][A-Za-z0-9_]*)(\s*=\s*)(.*?)(,?)(\s*)$`)
	closeRe = regexp.MustCompile(`^\s*\},?\s*$`)
)

// locate finds the line index of "Section.Key" (or a top-level "Key").
func locate(lines []string, path string) (int, error) {
	want := strings.Split(path, ".")
	var stack []string
	for i, l := range lines {
		t := strings.TrimRight(l, "\r\n")
		if strings.HasPrefix(strings.TrimSpace(t), "--") {
			continue
		}
		if m := openRe.FindStringSubmatch(t); m != nil {
			stack = append(stack, m[1])
			continue
		}
		if closeRe.MatchString(t) {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if m := keyRe.FindStringSubmatch(t); m != nil {
			// stack[0] is SandboxVars itself
			full := append(append([]string{}, stack[min(1, len(stack)):]...), m[2])
			if strings.Join(full, ".") == strings.Join(want, ".") {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("setting %q not found", path)
}

func Get(content, path string) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	i, err := locate(lines, path)
	if err != nil {
		return "", err
	}
	m := keyRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
	return m[4], nil
}

// Set replaces the value; value must be a Lua literal (3, true, "text").
func Set(content, path, value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n{}") {
		return "", fmt.Errorf("invalid value %q", value)
	}
	lines := strings.SplitAfter(content, "\n")
	i, err := locate(lines, path)
	if err != nil {
		return "", err
	}
	eol := ""
	switch {
	case strings.HasSuffix(lines[i], "\r\n"):
		eol = "\r\n"
	case strings.HasSuffix(lines[i], "\n"):
		eol = "\n"
	}
	m := keyRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
	lines[i] = m[1] + m[2] + m[3] + value + m[5] + eol
	return strings.Join(lines, ""), nil
}
