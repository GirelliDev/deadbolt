package cli

import (
	"flag"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/GirelliDev/deadbolt/internal/config"
	"github.com/GirelliDev/deadbolt/internal/rcon"
	"github.com/GirelliDev/deadbolt/internal/svc"
)

var playersRe = regexp.MustCompile(`Players connected \((\d+)\)`)

// env is what a server command needs, loaded once per invocation.
type env struct {
	cfg  config.Config
	ini  map[string]string
	unit svc.Unit
	rest []string // positional args left after flags

	force, reveal, all bool
	mod                string
}

// reorder moves flags before positional args so `config set K V -force` works.
// Flags that take a value (-config, -mod) bring their value along.
func reorder(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-" || a == "--" || len(a) < 2 || a[0] != '-':
			pos = append(pos, a)
		case (a == "-config" || a == "--config" || a == "-mod" || a == "--mod") && i+1 < len(args):
			flags = append(flags, a, args[i+1])
			i++
		default:
			flags = append(flags, a)
		}
	}
	return append(flags, pos...)
}

func load(name string, args []string, errw io.Writer) (*env, int) {
	fs := flag.NewFlagSet("dbolt "+name, flag.ContinueOnError)
	fs.SetOutput(errw)
	path := fs.String("config", "", "path to deadbolt.toml")
	force := fs.Bool("force", false, "edit even while the server is running")
	reveal := fs.Bool("reveal", false, "show secret values")
	all := fs.Bool("all", false, "mods add: enable every mod in the item")
	mod := fs.String("mod", "", "mods add: comma-separated mod ids to enable")
	if err := fs.Parse(reorder(args)); err != nil {
		if err == flag.ErrHelp {
			return nil, 0
		}
		return nil, 2
	}
	cfg, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return nil, 1
	}
	ini, err := config.ReadINI(cfg.INIPath())
	if err != nil {
		fmt.Fprintf(errw, "dbolt: cannot read %s: %v\n", cfg.INIPath(), err)
		return nil, 1
	}
	return &env{cfg: cfg, ini: ini, unit: svc.New(cfg.Server.Service), rest: fs.Args(), force: *force, reveal: *reveal, all: *all, mod: *mod}, -1
}

func (e *env) rcon() (*rcon.Client, error) {
	port, pass := e.cfg.RCON.Port, e.cfg.RCON.Password
	if port == 0 {
		port, _ = strconv.Atoi(e.ini["RCONPort"])
	}
	if pass == "" {
		pass = e.ini["RCONPassword"]
	}
	if port == 0 || pass == "" {
		return nil, fmt.Errorf("RCONPort/RCONPassword not set in %s", e.cfg.INIPath())
	}
	return rcon.Dial(fmt.Sprintf("%s:%d", e.cfg.RCON.Host, port), pass, 3*time.Second)
}

// players returns the online count, or -1 with an error if RCON is unreachable.
func (e *env) players() (int, error) {
	c, err := e.rcon()
	if err != nil {
		return -1, err
	}
	defer c.Close()
	out, err := c.Exec("players")
	if err != nil {
		return -1, err
	}
	m := playersRe.FindStringSubmatch(out)
	if m == nil {
		return -1, fmt.Errorf("unexpected players reply: %q", out)
	}
	return strconv.Atoi(m[1])
}

func cmdStatus(args []string, out, errw io.Writer) int {
	e, code := load("status", args, errw)
	if e == nil {
		return code
	}
	state := e.unit.State()
	fmt.Fprintf(out, "Server:   %s (%s) - %s\n", e.cfg.Server.Name, e.cfg.Server.Service, state)
	fmt.Fprintf(out, "Name:     %s\n", e.ini["PublicName"])
	fmt.Fprintf(out, "Port:     %s/udp\n", e.ini["DefaultPort"])
	fmt.Fprintf(out, "Mods:     %d (%d Workshop items)\n",
		config.ListCount(e.ini["Mods"]), config.ListCount(e.ini["WorkshopItems"]))
	if state != "active" {
		return 0
	}
	if n, err := e.players(); err != nil {
		fmt.Fprintf(out, "Players:  unknown (RCON: %v)\n", err)
	} else {
		fmt.Fprintf(out, "Players:  %d online\n", n)
	}
	return 0
}

func cmdStart(args []string, out, errw io.Writer) int {
	e, code := load("start", args, errw)
	if e == nil {
		return code
	}
	if err := e.unit.Start(); err != nil {
		fmt.Fprintln(errw, "dbolt: start failed:", err)
		return 1
	}
	fmt.Fprintln(out, "started", e.cfg.Server.Service)
	return 0
}

// saveWorld asks the running server to flush the world; best effort.
func (e *env) saveWorld(out io.Writer) {
	c, err := e.rcon()
	if err != nil {
		fmt.Fprintf(out, "warning: could not save through RCON (%v)\n", err)
		return
	}
	defer c.Close()
	if _, err := c.Exec("save"); err == nil {
		fmt.Fprintln(out, "world saved")
		time.Sleep(3 * time.Second)
	}
}

func cmdStop(args []string, out, errw io.Writer) int {
	e, code := load("stop", args, errw)
	if e == nil {
		return code
	}
	if e.unit.State() == "active" {
		e.saveWorld(out)
	}
	if err := e.unit.Stop(); err != nil {
		fmt.Fprintln(errw, "dbolt: stop failed:", err)
		return 1
	}
	fmt.Fprintln(out, "stopped", e.cfg.Server.Service)
	return 0
}

func cmdRestart(args []string, out, errw io.Writer) int {
	e, code := load("restart", args, errw)
	if e == nil {
		return code
	}
	if e.unit.State() == "active" {
		e.saveWorld(out)
	}
	if err := e.unit.Restart(); err != nil {
		fmt.Fprintln(errw, "dbolt: restart failed:", err)
		return 1
	}
	fmt.Fprintln(out, "restarted", e.cfg.Server.Service)
	return 0
}
