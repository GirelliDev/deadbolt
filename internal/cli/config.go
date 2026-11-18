package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GirelliDev/deadbolt/internal/config"
	"github.com/GirelliDev/deadbolt/internal/ini"
	"github.com/GirelliDev/deadbolt/internal/sandbox"
)

const configUsage = `Usage: dbolt config <subcommand> [-config file] [-force] [-reveal]

  show                       list server settings (secrets hidden)
  get <Key>                  print one setting, e.g. MaxPlayers
  set <Key> <value>          change a setting in <server>.ini
  sandbox get <Sec.Key>      read a gameplay setting, e.g. ZombieLore.Speed
  sandbox set <Sec.Key> <v>  change it (Lua literal: 3, true, "text")

Edits keep comments and line endings and leave <file>.deadbolt-bak behind.
They are refused while the server runs (it would overwrite them); -force
overrides that, and the change then applies on the next restart.
`

func isSecret(key string) bool { return strings.Contains(strings.ToLower(key), "password") }

func (e *env) show(key, val string) string {
	if isSecret(key) && !e.reveal && val != "" {
		return "***"
	}
	return val
}

func (e *env) guardRunning(errw io.Writer) bool {
	if st := e.unit.State(); (st == "active" || st == "activating") && !e.force {
		fmt.Fprintf(errw, "dbolt: %s is running and would overwrite this edit; run `dbolt stop` first (or -force to apply it on next restart)\n", e.cfg.Server.Service)
		return false
	}
	return true
}

func (e *env) sandboxPath() string {
	return filepath.Join(e.cfg.Server.DataDir, "Server", e.cfg.Server.Name+"_SandboxVars.lua")
}

func cmdConfig(args []string, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, configUsage)
		return 0
	}
	sub := args[0]
	e, code := load("config "+sub, args[1:], errw)
	if e == nil {
		return code
	}
	arg := func(n int) bool {
		if len(e.rest) != n {
			fmt.Fprint(errw, configUsage)
			return false
		}
		return true
	}
	switch sub {
	case "show":
		f, err := ini.Load(e.cfg.INIPath())
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		for _, kv := range f.Pairs() {
			v := e.show(kv[0], kv[1])
			if n := config.ListCount(v); len(v) > 70 && strings.Contains(v, ";") {
				v = fmt.Sprintf("<%d entries>", n)
			}
			fmt.Fprintf(out, "%s=%s\n", kv[0], v)
		}
	case "get":
		if !arg(1) {
			return 2
		}
		f, err := ini.Load(e.cfg.INIPath())
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		v, ok := f.Get(e.rest[0])
		if !ok {
			fmt.Fprintf(errw, "dbolt: %s not set in %s\n", e.rest[0], e.cfg.INIPath())
			return 1
		}
		fmt.Fprintln(out, e.show(e.rest[0], v))
	case "set":
		if !arg(2) {
			return 2
		}
		if !e.guardRunning(errw) {
			return 1
		}
		f, err := ini.Load(e.cfg.INIPath())
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		old, had := f.Get(e.rest[0])
		if err := f.Set(e.rest[0], e.rest[1]); err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		if err := f.Save(); err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		if !had {
			old = "(unset)"
		}
		fmt.Fprintf(out, "%s: %s -> %s\n", e.rest[0], e.show(e.rest[0], old), e.show(e.rest[0], e.rest[1]))
	case "sandbox":
		return e.sandboxCmd(out, errw)
	default:
		fmt.Fprintf(errw, "dbolt config: unknown subcommand %q\n\n%s", sub, configUsage)
		return 2
	}
	return 0
}

func (e *env) sandboxCmd(out, errw io.Writer) int {
	if len(e.rest) < 2 {
		fmt.Fprint(errw, configUsage)
		return 2
	}
	b, err := os.ReadFile(e.sandboxPath())
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	switch op, key := e.rest[0], e.rest[1]; {
	case op == "get" && len(e.rest) == 2:
		v, err := sandbox.Get(string(b), key)
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		fmt.Fprintln(out, v)
	case op == "set" && len(e.rest) == 3:
		if !e.guardRunning(errw) {
			return 1
		}
		old, _ := sandbox.Get(string(b), key)
		nb, err := sandbox.Set(string(b), key, e.rest[2])
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		if err := ini.WriteAtomic(e.sandboxPath(), []byte(nb)); err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		fmt.Fprintf(out, "%s: %s -> %s\n", key, old, e.rest[2])
	default:
		fmt.Fprint(errw, configUsage)
		return 2
	}
	return 0
}
