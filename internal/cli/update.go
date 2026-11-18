package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/GirelliDev/deadbolt/internal/backup"
	"github.com/GirelliDev/deadbolt/internal/steam"
	"github.com/GirelliDev/deadbolt/internal/updater"
)

const updateUsage = `Usage: dbolt update [check|watch] [-config file] [flags]

  dbolt update check     show whether the game or any Workshop mod has an update
  dbolt update           apply a pending update now, only if the server is empty
  dbolt update -wait     wait until the server is empty, then apply
  dbolt update -force    apply even with players online (warns them, waits 5 min)
  dbolt update watch     keep checking (-interval 10m) and apply when the server is empty

The server is backed up first and rolled back if it does not come back up.
Workshop mods are refreshed by the server itself when it boots.
`

type pending struct {
	game        bool
	have, want  string
	branch      string
	mods        []steam.ModUpdate
	modsUnknown error // Workshop API unreachable: games updates still work
}

func (p pending) any() bool { return p.game || len(p.mods) > 0 }

// steamRunner runs steamcmd as the owner of the install dir, so files keep the
// server user's ownership even when dbolt runs as root.
func (e *env) steamRunner(stream io.Writer) steam.Runner {
	return func(args ...string) (string, error) {
		name, full := e.cfg.Steam.SteamCMD, args
		if st, err := os.Stat(e.cfg.Server.InstallDir); err == nil {
			if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Geteuid() {
				if u, err := user.LookupId(strconv.Itoa(int(sys.Uid))); err == nil {
					if os.Geteuid() == 0 {
						name, full = "runuser", append([]string{"-u", u.Username, "--", e.cfg.Steam.SteamCMD}, args...)
					} else { // a narrow sudoers rule allows exactly this
						name, full = "sudo", append([]string{"-n", "-H", "-u", u.Username, e.cfg.Steam.SteamCMD}, args...)
					}
				}
			}
		}
		cmd := exec.Command(name, full...)
		var buf limitedBuf
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if stream != nil {
			cmd.Stdout = io.MultiWriter(&buf, stream)
		}
		err := cmd.Run()
		return buf.String(), err
	}
}

type limitedBuf struct{ b []byte }

func (l *limitedBuf) Write(p []byte) (int, error) {
	if len(l.b) < 4<<20 {
		l.b = append(l.b, p...)
	}
	return len(p), nil
}
func (l *limitedBuf) String() string { return string(l.b) }

func (e *env) check() (pending, error) {
	var p pending
	have, installedBranch, err := steam.Installed(e.cfg.Server.InstallDir)
	if err != nil {
		return p, fmt.Errorf("cannot read installed version: %w", err)
	}
	p.branch = e.cfg.Steam.Branch
	if p.branch == "" {
		p.branch = installedBranch
	}
	want, err := steam.Latest(e.steamRunner(nil), p.branch)
	if err != nil {
		return p, err
	}
	p.have, p.want, p.game = have, want, have != want

	ids := steam.SplitList(e.ini["WorkshopItems"])
	if len(ids) == 0 {
		return p, nil
	}
	local, err := steam.LocalMods(e.cfg.Server.InstallDir)
	if err != nil {
		p.modsUnknown = err
		return p, nil
	}
	remote, err := steam.RemoteMods(&http.Client{Timeout: 30 * time.Second}, "", ids)
	if err != nil {
		p.modsUnknown = err
		return p, nil
	}
	p.mods = steam.OutdatedMods(ids, local, remote)
	return p, nil
}

func (p pending) print(out io.Writer) {
	if p.game {
		fmt.Fprintf(out, "Game:  update available (%s -> %s, branch %s)\n", p.have, p.want, p.branch)
	} else {
		fmt.Fprintf(out, "Game:  up to date (build %s, branch %s)\n", p.have, p.branch)
	}
	switch {
	case p.modsUnknown != nil:
		fmt.Fprintf(out, "Mods:  could not check (%v)\n", p.modsUnknown)
	case len(p.mods) == 0:
		fmt.Fprintln(out, "Mods:  all up to date")
	default:
		fmt.Fprintf(out, "Mods:  %d to update\n", len(p.mods))
		for _, m := range p.mods {
			note := ""
			if m.NotLoaded {
				note = " (not downloaded yet)"
			}
			fmt.Fprintf(out, "  - %s %s%s\n", m.ID, m.Title, note)
		}
	}
}

func (e *env) deps(out io.Writer, p pending, sleep func(time.Duration)) updater.Deps {
	d := updater.Deps{
		Players: e.players,
		Active:  func() bool { return e.unit.State() == "active" },
		Stop:    e.unit.Stop,
		Start:   e.unit.Start,
		Backup: func() (string, error) {
			b, err := e.makeBackup("pre-update", out)
			return b.Path, err
		},
		Restore: func(path string) error {
			_, err := backup.Restore(path, e.cfg.Server.DataDir, time.Now())
			return err
		},
		Log:         func(f string, a ...any) { fmt.Fprintf(out, f+"\n", a...) },
		Sleep:       sleep,
		PollEvery:   time.Minute,
		EmptyChecks: 2,
		StartWait:   60 * time.Second,
	}
	if p.game {
		d.Install = func() error {
			_, err := e.steamRunner(out)(steam.InstallArgs(e.cfg.Server.InstallDir, p.branch, false)...)
			return err
		}
	}
	return d
}

func cmdUpdate(args []string, out, errw io.Writer) int {
	sub := ""
	if len(args) > 0 && (args[0] == "check" || args[0] == "watch") {
		sub, args = args[0], args[1:]
	}
	if len(args) > 0 && (args[0] == "help") {
		fmt.Fprint(out, updateUsage)
		return 0
	}
	// Extra flags live on a separate FlagSet; env.load parses -config.
	wait, force, interval := false, false, 10*time.Minute
	fs := flag.NewFlagSet("dbolt update", flag.ContinueOnError)
	fs.SetOutput(errw)
	fs.Usage = func() { fmt.Fprint(errw, updateUsage) }
	cfgPath := fs.String("config", "", "path to deadbolt.toml")
	fs.BoolVar(&wait, "wait", false, "wait for an empty server")
	fs.BoolVar(&force, "force", false, "update even with players online")
	fs.DurationVar(&interval, "interval", interval, "watch: time between checks")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	e, code := load("update", []string{"-config", *cfgPath}, errw)
	if e == nil {
		return code
	}
	switch sub {
	case "check":
		p, err := e.check()
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		p.print(out)
		return 0
	case "watch":
		return e.watch(out, errw, interval)
	}

	p, err := e.check()
	if err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	p.print(out)
	if !p.any() {
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d := e.deps(out, p, sleeper(ctx))
	cancelled := func() bool { return ctx.Err() != nil }

	if d.Active() {
		n, perr := e.players()
		switch {
		case wait:
			if err := updater.WaitEmpty(d, cancelled); err != nil {
				fmt.Fprintln(errw, "dbolt:", err)
				return 1
			}
		case perr != nil && !force:
			fmt.Fprintf(errw, "dbolt: cannot tell if anyone is online (%v); fix RCON or use -force\n", perr)
			return 1
		case perr == nil && n > 0 && !force:
			fmt.Fprintf(errw, "dbolt: %d player(s) online; use -wait to wait for an empty server, or -force\n", n)
			return 1
		case perr == nil && n > 0 && force:
			e.countdown(out, 5*time.Minute, d.Sleep)
		}
	}
	if err := updater.Apply(d); err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	return 0
}

func sleeper(ctx context.Context) func(time.Duration) {
	return func(d time.Duration) {
		select {
		case <-time.After(d):
		case <-ctx.Done():
		}
	}
}

// countdown warns players through RCON and waits; used only with -force.
func (e *env) countdown(out io.Writer, total time.Duration, sleep func(time.Duration)) {
	c, err := e.rcon()
	if err != nil {
		return
	}
	defer c.Close()
	fmt.Fprintf(out, "warning players, updating in %s\n", total)
	c.Exec(fmt.Sprintf(`servermsg "Server update in %d minutes. Please log out."`, int(total.Minutes())))
	sleep(total)
}

func (e *env) watch(out, errw io.Writer, interval time.Duration) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sleep := sleeper(ctx)
	logf := func(f string, a ...any) { fmt.Fprintf(out, time.Now().Format("15:04:05 ")+f+"\n", a...) }
	logf("watching for updates every %s", interval)
	for ctx.Err() == nil {
		p, err := e.check()
		switch {
		case err != nil:
			logf("check failed: %v", err)
		case p.any():
			logf("update pending (game=%v, mods=%d); waiting for an empty server", p.game, len(p.mods))
			d := e.deps(out, p, sleep)
			if !d.Active() {
				logf("server is stopped; not touching it")
			} else if err := updater.WaitEmpty(d, func() bool { return ctx.Err() != nil }); err != nil {
				logf("not updating: %v", err)
			} else if err := updater.Apply(d); err != nil {
				logf("update failed: %v", err)
			}
		}
		sleep(interval)
	}
	return 0
}

func cmdInstall(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("dbolt install", flag.ContinueOnError)
	fs.SetOutput(errw)
	cfgPath := fs.String("config", "", "path to deadbolt.toml")
	branch := fs.String("branch", "", "Steam branch: public (default) or unstable")
	validate := fs.Bool("validate", false, "verify every game file")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	e, code := load("install", []string{"-config", *cfgPath}, errw)
	if e == nil {
		return code
	}
	b := *branch
	if b == "" {
		b = e.cfg.Steam.Branch
	}
	if b == "" {
		b = "public"
	}
	if st := e.unit.State(); st == "active" {
		fmt.Fprintf(errw, "dbolt: %s is running; use `dbolt update` for a running server\n", e.cfg.Server.Service)
		return 1
	}
	if err := os.MkdirAll(e.cfg.Server.InstallDir, 0o755); err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	fmt.Fprintf(out, "installing app %s (branch %s) into %s\n", steam.AppID, b, e.cfg.Server.InstallDir)
	if _, err := e.steamRunner(out)(steam.InstallArgs(e.cfg.Server.InstallDir, b, *validate)...); err != nil {
		fmt.Fprintln(errw, "dbolt: steamcmd failed:", err)
		return 1
	}
	fmt.Fprintln(out, "done")
	return 0
}
