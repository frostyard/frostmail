package settings

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
)

// How long a change of mode waits on this computer's maild: mailctl's own
// limit is 30 s, the launcher's about 10 s after a slow portal.
const (
	stopTimeout  = 35 * time.Second
	startTimeout = 60 * time.Second
)

// The words of a change that failed (docs/specs/settings-ui.md).
const (
	didNotStop  = "This computer's maild did not stop. Frostmail still uses it."
	didNotStart = "This computer's maild did not start: "
)

// steps are the parts of a change of mode: the app's own configuration,
// and this computer's maild. The order lives in change, apart from the
// processes it runs, so that a test drives it with steps that only record
// their calls.
type steps interface {
	load() (conn.AppConfig, error)
	save(conn.AppConfig) error
	stopMaild() error
	startMaild() error
}

// outcome is what a change of mode did. moved says whether the session
// must go to the maild now chosen, whatever the outcome; err is the words
// the Server pane shows.
type outcome struct {
	moved bool
	err   string
}

// change moves the app's choice of maild to next, in the order ADR-0046
// fixes. Outside the Flatpak the app runs no maild of its own, so it only
// writes the choice. In the Flatpak each mode passes through this
// computer's maild:
//   - this computer to lodge: write the choice, then stop maild; when maild
//     does not stop, write the previous choice back, and leave the session
//     on this computer's maild;
//   - lodge to this computer: write the choice, then start maild, and move
//     the session either way;
//   - a new lodge address, or no change of mode: only write.
//
// The new address is checked before the first write, so a misspelled one
// changes nothing.
func change(s steps, flatpak bool, next conn.AppConfig) outcome {
	if next.Lodge != "" {
		if _, err := conn.ParseLodge(next.Lodge); err != nil {
			return outcome{err: err.Error()}
		}
	}
	if !flatpak {
		return writeOnly(s, next)
	}
	// The previous choice, before the first write: a failure here leaves
	// everything as it is, and nothing to write back.
	prev, err := s.load()
	if err != nil {
		return outcome{err: err.Error()}
	}
	switch {
	case prev.Lodge == "" && next.Lodge != "":
		return toLodge(s, prev, next)
	case prev.Lodge != "" && next.Lodge == "":
		return toLocal(s, next)
	}
	return writeOnly(s, next)
}

// writeOnly saves a choice that no maild of this computer depends on.
func writeOnly(s steps, next conn.AppConfig) outcome {
	if err := s.save(next); err != nil {
		return outcome{err: err.Error()}
	}
	return outcome{moved: true}
}

func toLodge(s steps, prev, next conn.AppConfig) outcome {
	if err := s.save(next); err != nil {
		return outcome{err: err.Error()}
	}
	if s.stopMaild() == nil {
		return outcome{moved: true}
	}
	// mailctl's own words do not say what the pane needs to: maild still
	// runs, and the previous choice stands.
	if err := s.save(prev); err != nil {
		return outcome{err: didNotStop + " " + err.Error()}
	}
	// A maild that took rpc.shutdown late can still exit: make sure this
	// computer's maild runs, as the mode says. The launcher keeps one that
	// answers, and its failure adds nothing to the pane's words.
	_ = s.startMaild()
	return outcome{err: didNotStop}
}

func toLocal(s steps, next conn.AppConfig) outcome {
	if err := s.save(next); err != nil {
		return outcome{err: err.Error()}
	}
	// The mode is local now: the session tries this computer's maild,
	// which starts or does not.
	if err := s.startMaild(); err != nil {
		return outcome{moved: true, err: didNotStart + err.Error()}
	}
	return outcome{moved: true}
}

// realSteps are the app's own steps, through Options.
type realSteps struct{ o *Options }

func (r realSteps) load() (conn.AppConfig, error) { return conn.ReadAppConfig(r.o.ConfigFile) }

func (r realSteps) save(cfg conn.AppConfig) error { return conn.WriteAppConfig(r.o.ConfigFile, cfg) }

func (r realSteps) stopMaild() error {
	if r.o.StopLocal == nil {
		return errors.New("cannot stop this computer's maild here")
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	return r.o.StopLocal(ctx)
}

func (r realSteps) startMaild() error {
	if r.o.StartLocal == nil {
		return errors.New("cannot start this computer's maild here")
	}
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	return r.o.StartLocal(ctx)
}

// targetFor says where the session goes for a choice.
func (w *Window) targetFor(lodge string) (conn.Target, error) {
	if lodge != "" {
		l, err := conn.ParseLodge(lodge)
		if err != nil {
			return conn.Target{}, err
		}
		return conn.Target{Lodge: &l}, nil
	}
	if w.o.Socket != "" {
		return conn.Target{Socket: w.o.Socket}, nil
	}
	if socket := os.Getenv("FROSTMAIL_SOCKET"); socket != "" {
		return conn.Target{Socket: socket}, nil
	}
	socket, err := config.DefaultSocket(os.Getenv)
	return conn.Target{Socket: socket}, err
}

// saveServer changes the mode on a worker. One change runs at a time: the
// pane holds Save back while one runs, and the session's target moves only
// at its end, so nothing connects to a choice that a failed stop writes
// back.
func (w *Window) saveServer(next string) {
	s := &w.server
	if s.busy {
		return
	}
	// A bad address is change's to refuse, as the first step of the save.
	target, err := w.targetFor(next)
	if err != nil && next == "" {
		s.err = err.Error()
		return
	}
	flatpak := w.facts.Flatpak
	s.busy, s.status, s.err = true, saveStatus(flatpak, s.stored, next), ""
	go func() {
		out := change(realSteps{&w.o}, flatpak, conn.AppConfig{Lodge: next})
		if out.moved && w.o.SetTarget != nil {
			w.o.SetTarget(target)
		}
		w.o.Post(func() {
			s.busy, s.status, s.err = false, "", out.err
			s.saved++
			w.loadFacts()
			w.readServer(true)
		})
	}()
}
