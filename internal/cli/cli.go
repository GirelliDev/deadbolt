// Package cli implements the dbolt command line.
package cli

import (
	"fmt"
	"io"
)

// Version is overridden at build time with -ldflags.
var Version = "0.0.1-dev"

type command struct {
	name, summary string
	run           func(args []string, out, errw io.Writer) int
}

func commands() []command {
	return []command{
		{"status", "show server state, players and pending updates", todo("status")},
		{"start", "start the server", todo("start")},
		{"stop", "stop the server (saves first)", todo("stop")},
		{"restart", "restart the server", todo("restart")},
		{"backup", "create, list and restore backups", todo("backup")},
		{"install", "install the server with SteamCMD", todo("install")},
		{"update", "update game and mods (waits for an empty server)", todo("update")},
		{"mods", "add, remove and list Workshop mods", todo("mods")},
		{"config", "view and edit server settings", todo("config")},
		{"web", "run the web panel", todo("web")},
	}
}

func todo(name string) func([]string, io.Writer, io.Writer) int {
	return func(_ []string, _, errw io.Writer) int {
		fmt.Fprintf(errw, "dbolt %s: not implemented yet\n", name)
		return 1
	}
}

// Run executes dbolt and returns the process exit code.
func Run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		usage(out)
		return 0
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		usage(out)
		return 0
	case "-v", "-version", "--version", "version":
		fmt.Fprintln(out, "dbolt", Version)
		return 0
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args[1:], out, errw)
		}
	}
	fmt.Fprintf(errw, "dbolt: unknown command %q\n\n", args[0])
	usage(errw)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Deadbolt - manage a Project Zomboid server.")
	fmt.Fprintln(w, "\nUsage: dbolt <command> [options]")
	fmt.Fprintln(w, "\nCommands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nFlags:\n  -h, -help, --help   show this help\n  -v, --version       show version")
}
