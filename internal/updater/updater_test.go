package updater

import (
	"errors"
	"testing"
	"time"
)

type fake struct {
	players        []int // successive answers; last repeats
	pErr           error
	calls          []string
	active         bool
	installErr     error
	startKeepsUp   bool
	backupErr      error
	sleeps, polled int
}

func (f *fake) deps() Deps {
	return Deps{
		Players: func() (int, error) {
			if f.pErr != nil {
				return -1, f.pErr
			}
			i := min(f.polled, len(f.players)-1)
			f.polled++
			return f.players[i], nil
		},
		Active:  func() bool { return f.active },
		Stop:    func() error { f.calls = append(f.calls, "stop"); f.active = false; return nil },
		Start:   func() error { f.calls = append(f.calls, "start"); f.active = f.startKeepsUp; return nil },
		Backup:  func() (string, error) { f.calls = append(f.calls, "backup"); return "a.tar.zst", f.backupErr },
		Restore: func(string) error { f.calls = append(f.calls, "restore"); return nil },
		Install: func() error { f.calls = append(f.calls, "install"); return f.installErr },
		Sleep:   func(time.Duration) { f.sleeps++ },

		EmptyChecks: 2,
	}
}

func TestWaitEmptyNeedsConsecutiveZeros(t *testing.T) {
	f := &fake{players: []int{3, 0, 1, 0, 0}}
	if err := WaitEmpty(f.deps(), nil); err != nil {
		t.Fatal(err)
	}
	if f.polled != 5 { // 3, 0 (streak1), 1 (reset), 0, 0 -> done
		t.Fatalf("polled %d", f.polled)
	}
}

func TestWaitEmptyAbortsWhenCountUnknown(t *testing.T) {
	f := &fake{pErr: errors.New("refused")}
	if err := WaitEmpty(f.deps(), nil); !errors.Is(err, ErrUnknownPlayers) {
		t.Fatalf("%v", err)
	}
}

func TestApplyHappyPath(t *testing.T) {
	f := &fake{active: true, startKeepsUp: true}
	if err := Apply(f.deps()); err != nil {
		t.Fatal(err)
	}
	want := []string{"backup", "stop", "install", "start"}
	if len(f.calls) != 4 || f.calls[0] != want[0] || f.calls[1] != want[1] || f.calls[2] != want[2] || f.calls[3] != want[3] {
		t.Fatalf("%v", f.calls)
	}
}

func TestApplyInstallFailureRollsBack(t *testing.T) {
	f := &fake{active: true, startKeepsUp: true, installErr: errors.New("disk full")}
	err := Apply(f.deps())
	if err == nil || f.calls[len(f.calls)-1] != "start" || f.calls[len(f.calls)-2] != "restore" {
		t.Fatalf("%v %v", err, f.calls)
	}
}

func TestApplyServerCrashesAfterStartRollsBack(t *testing.T) {
	f := &fake{active: true, startKeepsUp: false}
	d := f.deps()
	d.Start = func() error {
		f.calls = append(f.calls, "start")
		f.active = len(f.calls) > 6 // dies the first time, survives after rollback
		return nil
	}
	if err := Apply(d); err == nil {
		t.Fatal("must report the failed update")
	}
	if f.calls[len(f.calls)-2] != "restore" {
		t.Fatalf("%v", f.calls)
	}
}

func TestApplyBackupFailureAbortsBeforeStopping(t *testing.T) {
	f := &fake{active: true, backupErr: errors.New("no space")}
	if err := Apply(f.deps()); err == nil {
		t.Fatal("expected error")
	}
	if len(f.calls) != 1 {
		t.Fatalf("server must not be touched: %v", f.calls)
	}
}

func TestApplyModsOnlySkipsInstall(t *testing.T) {
	f := &fake{active: true, startKeepsUp: true}
	d := f.deps()
	d.Install = nil
	if err := Apply(d); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c == "install" {
			t.Fatal("install must be skipped")
		}
	}
}
