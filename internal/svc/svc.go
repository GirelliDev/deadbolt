// Package svc wraps systemctl for one unit.
package svc

import (
	"os"
	"os/exec"
	"strings"
)

type Unit struct {
	Name string
	// Run executes a command and returns combined output; replaceable in tests.
	Run func(name string, args ...string) (string, error)
}

func New(name string) Unit { return Unit{Name: name, Run: run} }

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (u Unit) ctl(args ...string) (string, error) {
	args = append(args, u.Name)
	if os.Geteuid() != 0 { // the deadbolt user gets a narrow sudoers rule
		return u.Run("sudo", append([]string{"-n", "systemctl"}, args...)...)
	}
	return u.Run("systemctl", args...)
}

// State returns systemctl's is-active word: active, inactive, failed, activating...
func (u Unit) State() string {
	out, _ := u.ctl("is-active") // exit code != 0 for inactive; the word is what matters
	if out == "" {
		return "unknown"
	}
	return strings.Fields(out)[0]
}

func (u Unit) Start() error   { _, err := u.ctl("start"); return err }
func (u Unit) Stop() error    { _, err := u.ctl("stop"); return err }
func (u Unit) Restart() error { _, err := u.ctl("restart"); return err }
