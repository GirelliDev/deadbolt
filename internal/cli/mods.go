package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GirelliDev/deadbolt/internal/ini"
	"github.com/GirelliDev/deadbolt/internal/mods"
	"github.com/GirelliDev/deadbolt/internal/steam"
)

const modsUsage = `Usage: dbolt mods <subcommand> [-config file] [-force]

  list                         show Workshop items and the mods they provide
  add <workshop-id> [-mod a,b | -all]
                               download an item and enable it
  remove <workshop-id>         disable an item and its mods (files stay on disk)

An item can hold several mods (often alternative variants), so "add" asks you
to pick with -mod or -all when there is more than one. Changes are refused
while the server runs; stop it first, then start it again.
`

func (e *env) itemDir(id string) string {
	return filepath.Join(e.cfg.Server.InstallDir, "steamapps", "workshop", "content", steam.WorkshopAppID, id)
}

func cmdMods(args []string, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, modsUsage)
		return 0
	}
	sub := args[0]
	e, code := load("mods "+sub, args[1:], errw)
	if e == nil {
		return code
	}
	switch sub {
	case "list":
		return e.modsList(out, errw)
	case "add", "remove":
		if len(e.rest) != 1 || !mods.ValidWorkshopID(e.rest[0]) {
			fmt.Fprintf(errw, "dbolt: give one numeric Workshop id\n\n%s", modsUsage)
			return 2
		}
		if !e.guardRunning(errw) {
			return 1
		}
		if sub == "add" {
			return e.modsAdd(e.rest[0], out, errw)
		}
		return e.modsRemove(e.rest[0], out, errw)
	}
	fmt.Fprintf(errw, "dbolt mods: unknown subcommand %q\n\n%s", sub, modsUsage)
	return 2
}

func (e *env) modsList(out, errw io.Writer) int {
	f, err := ini.Load(e.cfg.INIPath())
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	items, _ := f.Get("WorkshopItems")
	enabled, _ := f.Get("Mods")
	for _, id := range steam.SplitList(items) {
		ids, err := mods.ModIDs(e.itemDir(id))
		if err != nil {
			fmt.Fprintf(out, "%-12s (not downloaded)\n", id)
			continue
		}
		var parts []string
		for _, m := range ids {
			mark := "-"
			if mods.Has(enabled, m) {
				mark = "+"
			}
			parts = append(parts, mark+m)
		}
		fmt.Fprintf(out, "%-12s %s\n", id, strings.Join(parts, " "))
	}
	fmt.Fprintln(out, "\n+ enabled in Mods=   - provided by the item but not enabled")
	return 0
}

func (e *env) modsAdd(id string, out, errw io.Writer) int {
	if _, err := os.Stat(e.itemDir(id)); err != nil {
		fmt.Fprintf(out, "downloading Workshop item %s\n", id)
		if _, err := e.steamRunner(out)(steam.WorkshopArgs(e.cfg.Server.InstallDir, id)...); err != nil {
			fmt.Fprintln(errw, "dbolt: steamcmd failed:", err)
			return 1
		}
	}
	avail, err := mods.ModIDs(e.itemDir(id))
	if err != nil {
		fmt.Fprintf(errw, "dbolt: item %s not found after download (private, removed or not for this game?)\n", id)
		return 1
	}
	var pick []string
	switch {
	case e.mod != "":
		for _, m := range strings.Split(e.mod, ",") {
			if m = strings.TrimSpace(m); !mods.Has(strings.Join(avail, ";"), m) {
				fmt.Fprintf(errw, "dbolt: %q is not in item %s (available: %s)\n", m, id, strings.Join(avail, ", "))
				return 1
			}
			pick = append(pick, m)
		}
	case e.all || len(avail) == 1:
		pick = avail
	case len(avail) == 0:
		fmt.Fprintln(out, "note: this item has no mod.info (maybe a map or a collection); adding only the Workshop id")
	default:
		fmt.Fprintf(errw, "dbolt: item %s provides %d mods: %s\nnothing changed; choose with -mod <ids> or -all\n", id, len(avail), strings.Join(avail, ", "))
		return 2
	}
	f, err := ini.Load(e.cfg.INIPath())
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	items, _ := f.Get("WorkshopItems")
	cur, _ := f.Get("Mods")
	f.Set("WorkshopItems", mods.Add(items, id))
	f.Set("Mods", mods.Add(cur, pick...))
	if err := f.Save(); err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	fmt.Fprintf(out, "added item %s, enabled: %s\nrestart the server to load it (dbolt start)\n", id, strings.Join(pick, ", "))
	return 0
}

func (e *env) modsRemove(id string, out, errw io.Writer) int {
	f, err := ini.Load(e.cfg.INIPath())
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	items, _ := f.Get("WorkshopItems")
	if !mods.Has(items, id) {
		fmt.Fprintf(errw, "dbolt: item %s is not in WorkshopItems\n", id)
		return 1
	}
	cur, _ := f.Get("Mods")
	ids, err := mods.ModIDs(e.itemDir(id))
	if err != nil {
		fmt.Fprintln(out, "warning: item files not found, so its mod ids are unknown; remove them from Mods= by hand with `dbolt config set Mods ...`")
	}
	f.Set("WorkshopItems", mods.Remove(items, id))
	f.Set("Mods", mods.Remove(cur, ids...))
	if err := f.Save(); err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	fmt.Fprintf(out, "removed item %s and mods: %s\n", id, strings.Join(ids, ", "))
	return 0
}
