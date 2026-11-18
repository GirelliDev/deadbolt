// Package backup creates, lists, prunes and restores .tar.zst archives of the
// Project Zomboid data directory (Saves, Server settings, db).
package backup

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Top-level folders of the Zomboid user dir that make up a backup.
var Dirs = []string{"Saves", "Server", "db"}

const stamp = "20060102-150405"

var nameRe = regexp.MustCompile(`^(.+)-(\d{8}-\d{6})-([a-z-]+)\.tar\.zst$`)

type Info struct {
	Name   string
	Path   string
	Time   time.Time
	Reason string // manual, auto, pre-update...
	Size   int64
}

// Create archives the data dir into destDir and returns the new backup.
func Create(server, dataDir, destDir, reason string, now time.Time) (Info, error) {
	if !regexp.MustCompile(`^[a-z-]+$`).MatchString(reason) {
		return Info{}, fmt.Errorf("invalid backup reason %q", reason)
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return Info{}, err
	}
	name := fmt.Sprintf("%s-%s-%s.tar.zst", server, now.Format(stamp), reason)
	final := filepath.Join(destDir, name)
	tmp := filepath.Join(destDir, "."+name+".partial")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Info{}, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	zw, err := zstd.NewWriter(f)
	if err != nil {
		return Info{}, err
	}
	tw := tar.NewWriter(zw)
	found := 0
	for _, d := range Dirs {
		root := filepath.Join(dataDir, d)
		if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		found++
		if err := addTree(tw, dataDir, root); err != nil {
			return Info{}, err
		}
	}
	if found == 0 {
		return Info{}, fmt.Errorf("nothing to back up under %s (expected %s)", dataDir, strings.Join(Dirs, ", "))
	}
	if err := tw.Close(); err != nil {
		return Info{}, err
	}
	if err := zw.Close(); err != nil {
		return Info{}, err
	}
	if err := f.Close(); err != nil {
		return Info{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return Info{}, err
	}
	ok = true
	st, _ := os.Stat(final)
	return Info{Name: name, Path: final, Time: now, Reason: reason, Size: st.Size()}, nil
}

func addTree(tw *tar.Writer, base, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) { // file vanished while the server is running
				return nil
			}
			return err
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&fs.ModeSymlink == 0 {
			return nil // sockets, devices...
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		src, err := os.Open(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return errors.New("file vanished mid-backup: " + p + " (stop the server for a consistent backup)")
			}
			return err
		}
		defer src.Close()
		// The file may grow while the server writes; copy exactly the size in the header.
		_, err = io.CopyN(tw, src, hdr.Size)
		if err == io.EOF {
			err = errors.New("file shrank mid-backup: " + p)
		}
		return err
	})
}

// List returns this server's backups in dir, newest first.
func List(dir, server string) ([]Info, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range ents {
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil || m[1] != server {
			continue
		}
		t, err := time.ParseInLocation(stamp, m[2], time.Local)
		if err != nil {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Time: t, Reason: m[3], Size: st.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// Prune returns the backups to delete so that only the newest keep remain.
// keep < 1 is treated as 1: pruning never removes the last backup.
func Prune(list []Info, keep int) []Info {
	keep = max(keep, 1)
	sorted := append([]Info(nil), list...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.After(sorted[j].Time) })
	if len(sorted) <= keep {
		return nil
	}
	return sorted[keep:]
}

// Restore extracts archive into dataDir. Existing Saves/Server/db folders that
// the archive replaces are moved aside to <name>.pre-restore-<stamp>, never deleted.
// The server must be stopped by the caller. It returns the moved-aside paths.
func Restore(archive, dataDir string, now time.Time) ([]string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	tmp := filepath.Join(dataDir, ".deadbolt-restore-"+now.Format(stamp))
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	present := map[string]bool{}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		top, err := safeName(hdr.Name)
		if err != nil {
			return nil, err
		}
		present[top] = true
		target := filepath.Join(tmp, filepath.FromSlash(path.Clean(hdr.Name)))
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			err = writeFile(target, tr, fs.FileMode(hdr.Mode).Perm())
			if err == nil {
				os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			}
		case tar.TypeSymlink:
			if path.IsAbs(hdr.Linkname) || strings.HasPrefix(path.Clean(path.Join(path.Dir(hdr.Name), hdr.Linkname)), "..") {
				return nil, fmt.Errorf("unsafe symlink %q -> %q in archive", hdr.Name, hdr.Linkname)
			}
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			err = os.Symlink(hdr.Linkname, target)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	if len(present) == 0 {
		return nil, errors.New("archive is empty")
	}
	fixAccess(tmp, dataDir)

	var moved []string
	suffix := ".pre-restore-" + now.Format(stamp)
	for _, d := range Dirs {
		if !present[d] {
			continue
		}
		live := filepath.Join(dataDir, d)
		if _, err := os.Lstat(live); err == nil {
			if err := os.Rename(live, live+suffix); err != nil {
				return moved, err
			}
			moved = append(moved, live+suffix)
		}
		if err := os.Rename(filepath.Join(tmp, d), live); err != nil {
			return moved, fmt.Errorf("%w (previous data kept in %s)", err, strings.Join(moved, ", "))
		}
	}
	return moved, nil
}

func safeName(name string) (top string, err error) {
	c := path.Clean(name)
	if path.IsAbs(c) || c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("unsafe path %q in archive", name)
	}
	top = strings.SplitN(c, "/", 2)[0]
	for _, d := range Dirs {
		if top == d {
			return top, nil
		}
	}
	return "", fmt.Errorf("unexpected top-level entry %q in archive", name)
}

func writeFile(p string, r io.Reader, mode fs.FileMode) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fixAccess makes the restored tree usable by the (non-root) game user.
// As root it hands ownership to the owner of ref. As another user (dbolt) it
// adds group read/write so the ACL granted to the game user takes effect
// (the ACL mask follows the group bits).
func fixAccess(root, ref string) {
	if os.Geteuid() != 0 {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if info, err := d.Info(); err == nil {
				add := fs.FileMode(0o060)
				if info.IsDir() {
					add = 0o070
				}
				os.Chmod(p, info.Mode().Perm()|add)
			}
			return nil
		})
		return
	}
	st, err := os.Stat(ref)
	if err != nil {
		return
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			os.Lchown(p, int(sys.Uid), int(sys.Gid))
		}
		return nil
	})
}
