package cli

import "time"

// SetAskEvery changes how often connect asks a controller for a token, so a
// test of a button being held does not take as long as holding one, and
// returns the way back.
func SetAskEvery(d time.Duration) (restore func()) {
	old := askEvery
	askEvery = d

	return func() { askEvery = old }
}
