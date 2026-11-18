// Package config loads deadbolt.toml and the Project Zomboid server .ini.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Server struct {
	Name       string `toml:"name"`        // servername, e.g. "servertest"
	Service    string `toml:"service"`     // systemd unit
	InstallDir string `toml:"install_dir"` // where the dedicated server lives
	DataDir    string `toml:"data_dir"`    // the Zomboid user dir (Saves, Server, ...)
}

type RCON struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`     // 0 = read RCONPort from the server ini
	Password string `toml:"password"` // empty = read RCONPassword from the server ini
}

type Backup struct {
	Dir  string `toml:"dir"`
	Keep int    `toml:"keep"` // how many backups to retain; 1 = a new one replaces the old one
}

type Steam struct {
	SteamCMD string `toml:"steamcmd"` // path to the steamcmd binary
	Branch   string `toml:"branch"`   // "" = keep the installed branch; public, unstable...
}

type Web struct {
	Listen   string `toml:"listen"`    // loopback by default; anything else is opt-in
	StateDir string `toml:"state_dir"` // admin password hash and audit log (0700)
}

type Config struct {
	Server Server `toml:"server"`
	RCON   RCON   `toml:"rcon"`
	Backup Backup `toml:"backup"`
	Steam  Steam  `toml:"steam"`
	Web    Web    `toml:"web"`
}

func Default() Config {
	return Config{
		Server: Server{
			Name:       "servertest",
			Service:    "pz.service",
			InstallDir: "/home/pz/server",
			DataDir:    "/home/pz/Zomboid",
		},
		RCON:   RCON{Host: "127.0.0.1"},
		Backup: Backup{Dir: "/home/pz/deadbolt-backups", Keep: 1},
		Steam:  Steam{SteamCMD: "/usr/games/steamcmd"},
		Web:    Web{Listen: "127.0.0.1:8787", StateDir: "/var/lib/deadbolt"},
	}
}

// Load reads path; with an empty path it tries $DEADBOLT_CONFIG,
// ./deadbolt.toml and /etc/deadbolt/deadbolt.toml, falling back to defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	explicit := path != ""
	if !explicit {
		path = os.Getenv("DEADBOLT_CONFIG")
		explicit = path != ""
	}
	candidates := []string{path}
	if !explicit {
		candidates = []string{"deadbolt.toml", "/etc/deadbolt/deadbolt.toml"}
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		// Searching: a missing or unreadable candidate (e.g. ./deadbolt.toml in a
		// directory this user cannot read) just means "try the next one".
		if errors.Is(err, os.ErrNotExist) || (!explicit && errors.Is(err, os.ErrPermission)) {
			if explicit {
				return cfg, fmt.Errorf("config %s: %w", p, err)
			}
			continue
		}
		if err != nil {
			return cfg, err
		}
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config %s: %w", p, err)
		}
		break
	}
	return cfg, nil
}

// INIPath is the server settings file, e.g. <data>/Server/servertest.ini.
func (c Config) INIPath() string {
	return filepath.Join(c.Server.DataDir, "Server", c.Server.Name+".ini")
}

// ReadINI parses key=value lines. Lines starting with # are ignored.
func ReadINI(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20) // Mods= lines get long
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m, sc.Err()
}

// ListCount counts the entries of a ;-separated value (Mods, WorkshopItems).
func ListCount(v string) int {
	n := 0
	for _, s := range strings.Split(v, ";") {
		if strings.TrimSpace(s) != "" {
			n++
		}
	}
	return n
}
