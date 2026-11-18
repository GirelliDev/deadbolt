package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GirelliDev/deadbolt/internal/backup"
)

const backupUsage = `Usage: dbolt backup <subcommand> [-config file]

Subcommands:
  create             back up Saves, Server settings and db now
  list               list backups, newest first
  restore <name>     restore a backup (the server must be stopped;
                     replaced folders are kept as *.pre-restore-<time>)
  prune              delete backups outside the retention policy
`

func cmdBackup(args []string, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, backupUsage)
		return 0
	}
	sub := args[0]
	e, code := load("backup "+sub, args[1:], errw)
	if e == nil {
		return code
	}
	switch sub {
	case "create":
		if _, err := e.makeBackup("manual", out); err != nil {
			fmt.Fprintln(errw, "dbolt: backup failed:", err)
			return 1
		}
	case "list":
		list, err := backup.List(e.cfg.Backup.Dir, e.cfg.Server.Name)
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		if len(list) == 0 {
			fmt.Fprintln(out, "no backups in", e.cfg.Backup.Dir)
		}
		for _, b := range list {
			fmt.Fprintf(out, "%-52s %9s  %s\n", b.Name, human(b.Size), b.Reason)
		}
	case "restore":
		return e.restore(out, errw)
	case "prune":
		n, err := e.prune(out)
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		fmt.Fprintf(out, "pruned %d backup(s)\n", n)
	default:
		fmt.Fprintf(errw, "dbolt backup: unknown subcommand %q\n\n%s", sub, backupUsage)
		return 2
	}
	return 0
}

// makeBackup is shared with the update flow: it flushes the world when the
// server is running, archives the data dir and applies retention.
func (e *env) makeBackup(reason string, out io.Writer) (backup.Info, error) {
	if e.unit.State() == "active" {
		e.saveWorld(out)
	}
	b, err := backup.Create(e.cfg.Server.Name, e.cfg.Server.DataDir, e.cfg.Backup.Dir, reason, time.Now())
	if err != nil {
		return b, err
	}
	fmt.Fprintf(out, "backup created: %s (%s)\n", b.Path, human(b.Size))
	if _, err := e.prune(out); err != nil {
		fmt.Fprintln(out, "warning: prune failed:", err)
	}
	return b, nil
}

func (e *env) prune(out io.Writer) (int, error) {
	list, err := backup.List(e.cfg.Backup.Dir, e.cfg.Server.Name)
	if err != nil {
		return 0, err
	}
	del := backup.Prune(list, e.cfg.Backup.KeepDaily, e.cfg.Backup.KeepWeekly)
	for _, b := range del {
		if err := os.Remove(b.Path); err != nil {
			return 0, err
		}
	}
	return len(del), nil
}

func (e *env) restore(out, errw io.Writer) int {
	if len(e.rest) != 1 {
		fmt.Fprintln(errw, "usage: dbolt backup restore <name>")
		return 2
	}
	name := e.rest[0]
	if filepath.Base(name) != name { // only files inside the backup dir
		fmt.Fprintln(errw, "dbolt: give a backup name from `dbolt backup list`, not a path")
		return 2
	}
	if s := e.unit.State(); s == "active" || s == "activating" {
		fmt.Fprintf(errw, "dbolt: %s is %s; run `dbolt stop` before restoring\n", e.cfg.Server.Service, s)
		return 1
	}
	arch := filepath.Join(e.cfg.Backup.Dir, name)
	if !strings.HasSuffix(name, ".tar.zst") {
		arch += ".tar.zst"
	}
	moved, err := backup.Restore(arch, e.cfg.Server.DataDir, time.Now())
	if err != nil {
		fmt.Fprintln(errw, "dbolt: restore failed:", err)
		return 1
	}
	fmt.Fprintln(out, "restored", filepath.Base(arch))
	for _, m := range moved {
		fmt.Fprintln(out, "previous data kept at", m)
	}
	return 0
}

func human(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for m := n / u; m >= u; m /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
