// Package updater decides when it is safe to update and applies an update with
// backup and rollback. All side effects come in through Deps so the policy is testable.
package updater

import (
	"errors"
	"fmt"
	"time"
)

type Deps struct {
	Players func() (int, error) // online player count; error = unknown
	Active  func() bool
	Stop    func() error
	Start   func() error
	Backup  func() (archive string, err error)
	Restore func(archive string) error
	Install func() error // game update via steamcmd; nil when only mods changed
	Log     func(format string, a ...any)
	Sleep   func(time.Duration)

	PollEvery   time.Duration // between player checks
	EmptyChecks int           // consecutive empty checks required
	StartWait   time.Duration // how long the server must stay up after start
}

func (d Deps) log(f string, a ...any) {
	if d.Log != nil {
		d.Log(f, a...)
	}
}

// ErrUnknownPlayers means the player count could not be read, so we must not update.
var ErrUnknownPlayers = errors.New("cannot read player count (RCON unavailable)")

// WaitEmpty blocks until Players reports 0 for EmptyChecks polls in a row.
// If players join in between, the streak resets. It aborts on a read error:
// not knowing is treated as "someone may be playing".
func WaitEmpty(d Deps, cancelled func() bool) error {
	need := max(d.EmptyChecks, 1)
	streak := 0
	for {
		if cancelled != nil && cancelled() {
			return errors.New("cancelled")
		}
		n, err := d.Players()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnknownPlayers, err)
		}
		if n == 0 {
			streak++
			if streak >= need {
				return nil
			}
		} else {
			d.log("%d player(s) online, waiting", n)
			streak = 0
		}
		d.Sleep(d.PollEvery)
	}
}

// Apply backs up, stops, updates and starts the server. If the server does not
// stay up, it restores the backup and starts it again. The caller has already
// established the server is empty (or was forced).
func Apply(d Deps) error {
	arch, err := d.Backup()
	if err != nil {
		return fmt.Errorf("backup failed, update aborted: %w", err)
	}
	d.log("backup ready: %s", arch)
	wasActive := d.Active()
	if wasActive {
		if err := d.Stop(); err != nil {
			return fmt.Errorf("stop failed, update aborted: %w", err)
		}
	}
	if d.Install != nil {
		d.log("updating game files")
		if err := d.Install(); err != nil {
			return d.rollback(arch, fmt.Errorf("game update failed: %w", err), wasActive)
		}
	}
	if !wasActive { // it was stopped on purpose: update files, leave it stopped
		d.log("update applied (server was stopped, left stopped)")
		return nil
	}
	d.log("starting server (Workshop mods update on boot)")
	if err := d.Start(); err != nil {
		return d.rollback(arch, fmt.Errorf("start failed: %w", err), wasActive)
	}
	d.Sleep(d.StartWait)
	if !d.Active() {
		return d.rollback(arch, errors.New("server did not stay up after the update"), wasActive)
	}
	d.log("update applied")
	return nil
}

func (d Deps) rollback(arch string, cause error, restart bool) error {
	d.log("%v; restoring backup %s", cause, arch)
	_ = d.Stop() // may already be down
	if err := d.Restore(arch); err != nil {
		return fmt.Errorf("%w; ROLLBACK FAILED (backup at %s): %v", cause, arch, err)
	}
	if !restart {
		return fmt.Errorf("%w; rolled back to the backup", cause)
	}
	if err := d.Start(); err != nil {
		return fmt.Errorf("%w; data restored but server failed to start: %v", cause, err)
	}
	return fmt.Errorf("%w; rolled back to the backup (game files stay at the new version)", cause)
}
