package cli

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/GirelliDev/deadbolt/internal/backup"
	"github.com/GirelliDev/deadbolt/internal/config"
	"github.com/GirelliDev/deadbolt/internal/mods"
	"github.com/GirelliDev/deadbolt/internal/steam"
	"github.com/GirelliDev/deadbolt/internal/web"
)

const webUsage = `Usage: dbolt web [-config file] [-listen addr]
       dbolt web passwd [-config file] [-stdin]

  dbolt web           run the admin panel (default http://127.0.0.1:8787)
  dbolt web passwd    set the admin password: generates a strong one and prints it
                      once (-stdin reads the password from standard input instead)

The panel listens on loopback by default. To reach it from elsewhere, put an
HTTPS reverse proxy in front of it, or set -listen / [web] listen on purpose.
`

func cmdWeb(args []string, out, errw io.Writer) int {
	passwd := false
	if len(args) > 0 && args[0] == "passwd" {
		passwd, args = true, args[1:]
	}
	if len(args) > 0 && (args[0] == "help") {
		fmt.Fprint(out, webUsage)
		return 0
	}
	fs := flag.NewFlagSet("dbolt web", flag.ContinueOnError)
	fs.SetOutput(errw)
	fs.Usage = func() { fmt.Fprint(errw, webUsage) }
	cfgPath := fs.String("config", "", "path to deadbolt.toml")
	listen := fs.String("listen", "", "address to listen on (default from config)")
	stdin := fs.Bool("stdin", false, "passwd: read the password from stdin")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	e, code := load("web", []string{"-config", *cfgPath}, errw)
	if e == nil {
		return code
	}
	state := e.cfg.Web.StateDir
	if passwd {
		return setPassword(state, *stdin, out, errw)
	}
	addr := e.cfg.Web.Listen
	if *listen != "" {
		addr = *listen
	}
	if _, err := os.Stat(filepath.Join(state, "admin.hash")); err != nil {
		fmt.Fprintf(errw, "dbolt: no admin password yet; run `dbolt web passwd` first\n")
		return 1
	}
	if h, _, err := net.SplitHostPort(addr); err != nil || (h != "localhost" && !net.ParseIP(h).IsLoopback()) {
		fmt.Fprintf(errw, "warning: %s is not loopback; traffic is plain HTTP unless an HTTPS proxy is in front\n", addr)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           web.New(e.backend(), state, addr).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute, // update checks call steamcmd
		IdleTimeout:       time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); srv.Shutdown(context.Background()) }()
	fmt.Fprintf(out, "Deadbolt panel on http://%s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	return 0
}

func setPassword(state string, fromStdin bool, out, errw io.Writer) int {
	var pw string
	if fromStdin {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if err != nil {
			fmt.Fprintln(errw, "dbolt:", err)
			return 1
		}
		pw = strings.TrimRight(string(b), "\r\n")
	} else {
		const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		b := make([]byte, 20)
		rand.Read(b)
		for i := range b {
			b[i] = alphabet[int(b[i])%len(alphabet)]
		}
		pw = string(b)
	}
	if err := web.SetPassword(state, pw); err != nil {
		fmt.Fprintln(errw, "dbolt:", err)
		return 1
	}
	if fromStdin {
		fmt.Fprintln(out, "admin password updated")
	} else {
		fmt.Fprintf(out, "Admin password (shown once): %s\n", pw)
	}
	return 0
}

// backend adapts the server commands to what the panel needs. The ini is read
// fresh on every call because the panel is long-lived and settings can change.
func (e *env) backend() web.Backend {
	ini := func() map[string]string {
		m, err := config.ReadINI(e.cfg.INIPath())
		if err != nil {
			return map[string]string{}
		}
		return m
	}
	return web.Backend{
		Status: func() web.Status {
			m := ini()
			return web.Status{Server: e.cfg.Server.Name, Service: e.cfg.Server.Service, State: e.unit.State(),
				PublicName: m["PublicName"], Port: m["DefaultPort"],
				Mods: config.ListCount(m["Mods"]), Items: config.ListCount(m["WorkshopItems"])}
		},
		Rcon: func(cmd string) (string, error) {
			e.ini = ini()
			c, err := e.rcon()
			if err != nil {
				return "", err
			}
			defer c.Close()
			return c.Exec(cmd)
		},
		Control: func(action string) error {
			switch action {
			case "start":
				return e.unit.Start()
			case "stop":
				e.saveWorld(io.Discard)
				return e.unit.Stop()
			}
			e.saveWorld(io.Discard)
			return e.unit.Restart()
		},
		Backups: func() ([]web.BackupRow, error) {
			l, err := backup.List(e.cfg.Backup.Dir, e.cfg.Server.Name)
			rows := make([]web.BackupRow, 0, len(l))
			for _, b := range l {
				rows = append(rows, web.BackupRow{Name: b.Name, Size: b.Size, Reason: b.Reason, Time: b.Time.Format(time.RFC3339)})
			}
			return rows, err
		},
		CreateBackup: func() (string, error) {
			b, err := e.makeBackup("manual", io.Discard)
			return b.Name, err
		},
		Updates: func() (web.UpdateInfo, error) {
			e.ini = ini()
			p, err := e.check()
			if err != nil {
				return web.UpdateInfo{}, err
			}
			u := web.UpdateInfo{Mods: []string{}}
			if p.game {
				u.Game = fmt.Sprintf("build %s -> %s (%s)", p.have, p.want, p.branch)
			}
			for _, m := range p.mods {
				u.Mods = append(u.Mods, m.ID+" "+m.Title)
			}
			if p.modsUnknown != nil {
				u.Note = "Could not check Workshop mods: " + p.modsUnknown.Error()
			}
			return u, nil
		},
		Mods: func() ([]web.ModRow, error) {
			m := ini()
			enabled := m["Mods"]
			var rows []web.ModRow
			for _, id := range steam.SplitList(m["WorkshopItems"]) {
				ids, _ := mods.ModIDs(e.itemDir(id))
				var on []string
				for _, x := range ids {
					if mods.Has(enabled, x) {
						on = append(on, x)
					}
				}
				rows = append(rows, web.ModRow{ID: id, Mods: nonNil(ids), Enabled: nonNil(on)})
			}
			return rows, nil
		},
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
