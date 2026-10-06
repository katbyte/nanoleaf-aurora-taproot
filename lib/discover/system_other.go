//go:build !darwin

package discover

import "context"

// systemBrowse is the search through the system's own discovery service,
// which only a Mac needs: see system_darwin.go.
func systemBrowse(context.Context) []Found { return nil }
