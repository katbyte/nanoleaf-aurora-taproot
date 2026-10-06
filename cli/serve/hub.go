package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// hub listens to every controller's own event stream and tells the open pages
// when one of them changes, so a scene started from a phone, a wall switch or
// the controller's button shows up without waiting for the next poll.
type hub struct {
	mu      sync.Mutex
	ctx     context.Context //nolint:containedctx // the server's lifetime, which every listener it starts shares
	watched map[string]context.CancelFunc
	pages   map[chan string]bool
}

func newHub() *hub {
	return &hub{watched: map[string]context.CancelFunc{}, pages: map[chan string]bool{}}
}

// start gives the hub the context its listeners live under and starts one for
// each controller already known.
func (h *hub) start(ctx context.Context, s *server) {
	h.mu.Lock()
	h.ctx = ctx
	h.mu.Unlock()

	if st, err := s.f.OpenStore(); err == nil {
		h.sync(s, st.Controllers) //nolint:contextcheck // it reads the context just stored
	}
}

// lifetime is the context the server lives under: the one its listeners
// share, or one that never ends for a server that was never started.
func (h *hub) lifetime() context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx == nil {
		return context.Background()
	}

	return h.ctx
}

// sync starts listening to controllers that have appeared and stops
// listening to ones that have gone. A controller is known by its name, where
// it is and its token, so one that moved or was connected again is listened
// to afresh.
func (h *hub) sync(s *server, controllers []store.Controller) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx == nil {
		return // a server that was never started, as in a test of one handler
	}

	want := map[string]store.Controller{}
	for _, c := range controllers {
		want[c.Name+"\x00"+c.Host+"\x00"+c.Token] = c
	}
	for key, cancel := range h.watched {
		if _, ok := want[key]; !ok {
			cancel()
			delete(h.watched, key)
		}
	}
	for key, ctl := range want {
		if _, ok := h.watched[key]; ok {
			continue
		}
		c, err := s.f.Client(ctl)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithCancel(h.ctx)
		h.watched[key] = cancel
		go h.listen(ctx, s, ctl.Name, c)
	}
}

// listen holds one controller's event stream open, opening it again when it
// drops: quickly at first, then every half minute for a controller that is
// off or away.
func (h *hub) listen(ctx context.Context, s *server, name string, c *aurora.Client) {
	wait := 2 * time.Second
	for ctx.Err() == nil {
		opened := time.Now()
		err := c.Events(ctx, nil, func(aurora.Event) {
			s.stale()
			h.tell(name)
		})
		if ctx.Err() != nil {
			return
		}
		clog.Log.Debugf("%s: the event stream ended: %v", name, err)

		// a stream that held for a while was a working one: start patient again
		if time.Since(opened) > time.Minute {
			wait = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

// tell lets every open page know a controller changed.
func (h *hub) tell(controller string) {
	msg, err := json.Marshal(map[string]string{"controller": controller})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.pages {
		select {
		case ch <- string(msg):
		default: // a page that is not reading will catch up on its next poll
		}
	}
}

// serve is the page's own event stream: a line each time a controller
// changes, and a comment now and then so nothing in between closes it.
func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // a proxy in front must pass each line on as it comes
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan string, 16)
	h.mu.Lock()
	h.pages[ch] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pages, ch)
		h.mu.Unlock()
	}()

	beat := time.NewTicker(25 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": still here\n\n"); err != nil {
				return
			}
		}
		flusher.Flush()
	}
}
