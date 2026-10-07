package cli

import (
	"context"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/store"
)

// SetSearch replaces how connect looks for controllers with a list of canned
// ones, since what is on the network is not a test's to say, and returns the
// way back. Each is marked connected or not as a real search would mark it.
func SetSearch(found ...FoundController) (restore func()) {
	old := search
	search = func(_ context.Context, s *store.Store, _ time.Duration, _ string) ([]FoundController, error) {
		out := make([]FoundController, len(found))
		for i, c := range found {
			out[i] = c
			for _, saved := range s.Controllers {
				if saved.Host == c.Address {
					out[i].Connected = saved.Name
				}
			}
		}
		return out, nil
	}

	return func() { search = old }
}

// SetFirmwarePoll changes how often a controller being upgraded is asked
// whether it is back, so a test need not wait as an upgrade takes.
func SetFirmwarePoll(d time.Duration) (restore func()) {
	old := firmwarePoll
	firmwarePoll = d

	return func() { firmwarePoll = old }
}

// SetAskEvery changes how often connect asks a controller for a token, so a
// test of a button being held does not take as long as holding one, and
// returns the way back.
func SetAskEvery(d time.Duration) (restore func()) {
	old := askEvery
	askEvery = d

	return func() { askEvery = old }
}
