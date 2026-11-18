// Package mods works out which mod ids a Workshop item provides and edits the
// ;-separated Mods= / WorkshopItems= lists.
package mods

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var numericRe = regexp.MustCompile(`^[0-9]{5,}$`)

// ValidWorkshopID reports whether s looks like a Steam Workshop file id.
func ValidWorkshopID(s string) bool { return numericRe.MatchString(s) }

// IDsInFile returns the id= value of a mod.info file.
func IDsInFile(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok && strings.EqualFold(strings.TrimSpace(k), "id") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ModIDs lists the distinct mod ids in a downloaded Workshop item directory.
func ModIDs(itemDir string) ([]string, error) {
	if _, err := os.Stat(itemDir); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(itemDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "mod.info" {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil {
			if id := IDsInFile(b); id != "" {
				seen[id] = true
			}
		}
		return nil
	})
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, err
}

func split(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Add appends items that are not already present, keeping existing order.
func Add(list string, items ...string) string {
	cur := split(list)
	have := map[string]bool{}
	for _, c := range cur {
		have[c] = true
	}
	for _, it := range items {
		if it != "" && !have[it] {
			cur = append(cur, it)
			have[it] = true
		}
	}
	return strings.Join(cur, ";")
}

// Remove drops every occurrence of items.
func Remove(list string, items ...string) string {
	drop := map[string]bool{}
	for _, it := range items {
		drop[it] = true
	}
	var out []string
	for _, c := range split(list) {
		if !drop[c] {
			out = append(out, c)
		}
	}
	return strings.Join(out, ";")
}

func Has(list, item string) bool {
	for _, c := range split(list) {
		if c == item {
			return true
		}
	}
	return false
}
