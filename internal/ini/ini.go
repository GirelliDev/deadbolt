// Package ini edits Project Zomboid server .ini files line by line, keeping
// comments, key order and each line's own CRLF/LF ending.
package ini

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type File struct {
	path  string
	lines []string // each keeps its own line ending
	eol   string   // dominant ending, used for appended lines
}

func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &File{path: path, eol: "\n"}
	if bytes.Count(b, []byte("\r\n")) > bytes.Count(b, []byte("\n"))/2 {
		f.eol = "\r\n"
	}
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			f.lines = append(f.lines, string(b))
			break
		}
		f.lines = append(f.lines, string(b[:i+1]))
		b = b[i+1:]
	}
	return f, nil
}

func split(line string) (key, val string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	k, v, ok := strings.Cut(t, "=")
	return strings.TrimSpace(k), strings.TrimSpace(v), ok
}

func (f *File) Get(key string) (string, bool) {
	for _, l := range f.lines {
		if k, v, ok := split(l); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func ending(line string) string {
	switch {
	case strings.HasSuffix(line, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(line, "\n"):
		return "\n"
	}
	return ""
}

// Set replaces the value of key, or appends the key when it is missing.
func (f *File) Set(key, val string) error {
	if key == "" || strings.ContainsAny(key, "=\r\n") || strings.ContainsAny(val, "\r\n") {
		return fmt.Errorf("invalid key or value")
	}
	for i, l := range f.lines {
		if k, _, ok := split(l); ok && k == key {
			e := ending(l)
			if e == "" {
				e = f.eol
			}
			f.lines[i] = key + "=" + val + e
			return nil
		}
	}
	if n := len(f.lines); n > 0 && ending(f.lines[n-1]) == "" {
		f.lines[n-1] += f.eol
	}
	f.lines = append(f.lines, key+"="+val+f.eol)
	return nil
}

// Pairs returns every key=value line in file order.
func (f *File) Pairs() [][2]string {
	var out [][2]string
	for _, l := range f.lines {
		if k, v, ok := split(l); ok {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

func (f *File) Bytes() []byte { return []byte(strings.Join(f.lines, "")) }

// Save keeps a copy of the previous content next to the file (<name>.deadbolt-bak)
// and replaces the file atomically, preserving mode and owner.
func (f *File) Save() error { return WriteAtomic(f.path, f.Bytes()) }

func WriteAtomic(path string, data []byte) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".deadbolt-bak", old, 0o600); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".deadbolt-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
		tmp.Chown(int(sys.Uid), int(sys.Gid))
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
