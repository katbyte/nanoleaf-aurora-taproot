package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/cli"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// how long the page's connect keeps asking a controller for a token, and how often
var (
	pairFor   = 5 * time.Minute
	pairEvery = 2 * time.Second
)

// how long a finished connect stays listed, so the page gets to say how it went
const pairShown = 90 * time.Second

// The states of a pairing.
const (
	pairWaiting   = "waiting"   // asking, until somebody holds the button
	pairConnected = "connected" // a token was handed out and saved
	pairFailed    = "failed"    // it could not be reached, or nobody held the button in time
)

// pairing is one controller the page is connecting to.
type pairing struct {
	Address string    `json:"address"`
	State   string    `json:"state"`
	Message string    `json:"message,omitempty"`
	Until   time.Time `json:"until"` // when the asking stops
	Name    string    `json:"name,omitempty"`

	done   time.Time
	cancel context.CancelFunc
}

// pairings is the connects in progress and just finished, by address.
type pairings struct {
	mu   sync.Mutex
	jobs map[string]*pairing
}

func newPairings() *pairings { return &pairings{jobs: map[string]*pairing{}} }

// list is every connect the page should still show, oldest first.
func (p *pairings) list() []pairing {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]pairing, 0, len(p.jobs))
	for addr, j := range p.jobs {
		if j.State != pairWaiting && time.Since(j.done) > pairShown {
			delete(p.jobs, addr)
			continue
		}
		out = append(out, *j)
	}
	slices.SortFunc(out, func(a, b pairing) int { return a.Until.Compare(b.Until) })

	return out
}

// finish records how a connect ended.
func (p *pairings) finish(addr, state, message, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if j, ok := p.jobs[addr]; ok {
		j.State, j.Message, j.Name, j.done = state, message, name, time.Now()
	}
}

// postConnect starts asking a controller for a token. It answers at once: the
// asking goes on behind it until somebody holds the controller's button, and
// the page watches how it is going through the state it already polls.
func (s *server) postConnect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}
	if err := read(r, &in); err != nil {
		failFor(w, err)
		return
	}
	addr := strings.TrimSpace(in.Address)
	bare, err := s.f.Client(store.Controller{Name: addr, Host: addr})
	if err != nil {
		failFor(w, errors.Join(errBadRequest, err))
		return
	}

	s.pairs.mu.Lock()
	if j, ok := s.pairs.jobs[addr]; ok && j.State == pairWaiting {
		s.pairs.mu.Unlock()
		reply(w, http.StatusAccepted, *j) // already asking: one press serves both
		return
	}
	job := &pairing{Address: addr, State: pairWaiting, Until: time.Now().Add(pairFor)}
	s.pairs.jobs[addr] = job
	answer := *job
	s.pairs.mu.Unlock()

	go s.pair(job, bare, strings.TrimSpace(in.Name)) //nolint:contextcheck // the asking outlives this request
	reply(w, http.StatusAccepted, answer)
}

// pair asks a controller for a token until it hands one over, then saves it.
// It lives as long as the server does, or until the page stops it.
func (s *server) pair(job *pairing, bare *aurora.Client, name string) {
	ctx, cancel := context.WithCancel(s.hub.lifetime())
	defer cancel()
	s.pairs.mu.Lock()
	job.cancel = cancel
	addr := job.Address
	s.pairs.mu.Unlock()

	token, err := cli.AskForToken(ctx, bare, pairFor, pairEvery, s.f.DryRun, func(bool) {})
	if err != nil {
		s.pairs.finish(addr, pairFailed, err.Error(), "")
		return
	}
	if s.f.DryRun {
		s.pairs.finish(addr, pairFailed, "a dry run asks for no token", "")
		return
	}

	s.file.Lock()
	saved, err := s.f.Adopt(ctx, addr, token, name, true)
	s.file.Unlock()
	if err != nil {
		s.pairs.finish(addr, pairFailed, err.Error(), "")
		return
	}
	s.pairs.finish(addr, pairConnected, fmt.Sprintf("%s, firmware %s, %d panels, %d scenes", saved.Model, saved.Firmware, saved.Panels, saved.Scenes), saved.Name)
	s.stale()
}

// deleteConnect stops asking a controller for a token.
func (s *server) deleteConnect(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")

	s.pairs.mu.Lock()
	j, ok := s.pairs.jobs[addr]
	if ok {
		delete(s.pairs.jobs, addr)
	}
	s.pairs.mu.Unlock()
	if ok && j.cancel != nil {
		j.cancel()
	}
	w.WriteHeader(http.StatusNoContent)
}
