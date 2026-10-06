// Package store keeps the controllers taproot holds a token for, in one file
// that only its owner can read. A token never expires and gives full control
// of a controller, so it is treated as a password: it is written nowhere but
// here, and nothing in this package prints one.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// File is the name of the file inside the configuration directory.
const File = "controllers.json"

// Controller is one controller and the token that opens it.
type Controller struct {
	// Name is what it is called on the command line.
	Name string `json:"name"`
	// Device is what it calls itself: "Light Panels 53:A6:3C".
	Device string `json:"device"`
	// Host is its address: an IP or a host name, with a port when it is not
	// the usual one.
	Host     string    `json:"host"`
	Serial   string    `json:"serialNo,omitempty"`
	Model    string    `json:"model,omitempty"`
	Firmware string    `json:"firmwareVersion,omitempty"`
	Token    string    `json:"token"`
	Added    time.Time `json:"added"`
}

// Store is the file's contents.
type Store struct {
	path        string
	Controllers []Controller `json:"controllers"`
}

// DefaultDir is where taproot keeps its files unless told otherwise:
// ~/.config/taproot on every platform, so the path in the documentation is
// the path on disk.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "taproot")
	}

	return filepath.Join(home, ".config", "taproot")
}

// Open reads the store in dir. A directory or file that is not there yet is
// an empty store.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, File)}

	data, err := os.ReadFile(s.path) // the controllers file, in the directory the person named
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the controllers file: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("the controllers file %s is not valid: %w", s.path, err)
	}

	// a file that was copied or restored may have come back readable by others: close it again
	if info, err := os.Stat(s.path); err == nil && info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(s.path, 0o600) // Save sets it properly; this is only the sooner of the two
	}

	return s, nil
}

// Path is the file the store is read from and saved to.
func (s *Store) Path() string { return s.path }

// Save writes the store, readable by its owner alone, replacing the file in
// one step so a crash part way cannot leave half a file of tokens.
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	slices.SortFunc(s.Controllers, func(a, b Controller) int { return strings.Compare(a.Name, b.Name) })
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".controllers-*")
	if err != nil {
		return fmt.Errorf("saving the controllers file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // nothing to remove once the rename has happened

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("saving the controllers file: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("saving the controllers file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving the controllers file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("saving the controllers file: %w", err)
	}

	return nil
}

// Put adds a controller, or replaces the one that is the same device: the
// same serial number, or failing that the same address. It reports the name
// the controller went in under and whether it replaced one. The name is
// c.Name; with none given, the name the device already had, or else the one
// it gives itself; and with a number on the end when another device has it.
func (s *Store) Put(c Controller) (name string, replaced bool) {
	i := slices.IndexFunc(s.Controllers, func(o Controller) bool {
		if c.Serial != "" && o.Serial != "" {
			return c.Serial == o.Serial
		}
		return sameHost(c.Host, o.Host)
	})

	// connected again without being named again, it keeps the name it had
	if c.Name == "" && i >= 0 {
		c.Name = s.Controllers[i].Name
	}
	if c.Name == "" {
		c.Name = Slug(c.Device)
	}
	if c.Name == "" {
		c.Name = Slug(c.Host)
	}
	// two devices cannot share a name: the second gets a number
	base := c.Name
	for n := 2; slices.ContainsFunc(s.Controllers, func(o Controller) bool {
		return o.Name == c.Name && (i < 0 || o.Name != s.Controllers[i].Name)
	}); n++ {
		c.Name = fmt.Sprintf("%s-%d", base, n)
	}

	if i >= 0 {
		s.Controllers[i] = c
		return c.Name, true
	}
	s.Controllers = append(s.Controllers, c)

	return c.Name, false
}

// Remove takes a controller out by its name and reports whether it was there.
func (s *Store) Remove(name string) bool {
	before := len(s.Controllers)
	s.Controllers = slices.DeleteFunc(s.Controllers, func(c Controller) bool { return c.Name == name })

	return len(s.Controllers) != before
}

// Names lists the controllers' names.
func (s *Store) Names() []string {
	names := make([]string, len(s.Controllers))
	for i, c := range s.Controllers {
		names[i] = c.Name
	}
	slices.Sort(names)

	return names
}

// Find is the controller a person means by ref: its name, what it calls
// itself, its address, its serial number, or any part of those that only one
// controller has. Case and punctuation are ignored, so "53a63c" finds
// "Light Panels 53:A6:3C" and "183" finds 10.0.5.183.
func (s *Store) Find(ref string) (Controller, error) {
	if len(s.Controllers) == 0 {
		return Controller{}, errors.New("no controllers yet: taproot connect <address> gets a token for one")
	}
	want := fold(ref)
	if want == "" {
		return Controller{}, fmt.Errorf("which controller? one of: %s", strings.Join(s.Names(), ", "))
	}

	// a whole match beats a partial one: "bed" is not ambiguous when there is a "bed" and a "bedroom"
	for _, whole := range []bool{true, false} {
		var hits []Controller
		for _, c := range s.Controllers {
			if slices.ContainsFunc([]string{c.Name, c.Device, c.Host, c.Serial}, func(field string) bool {
				f := fold(field)
				if whole {
					return f == want
				}
				return f != "" && strings.Contains(f, want)
			}) {
				hits = append(hits, c)
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil
		default:
			names := make([]string, len(hits))
			for i, c := range hits {
				names[i] = c.Name
			}
			return Controller{}, fmt.Errorf("%q matches several controllers: %s", ref, strings.Join(names, ", "))
		}
	}

	return Controller{}, fmt.Errorf("no controller matches %q: taproot has %s", ref, strings.Join(s.Names(), ", "))
}

var notAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slug is a name made safe for the command line and for a directory: lower
// case, with everything but letters and digits turned into single dashes.
func Slug(s string) string {
	return strings.Trim(notAlnum.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// fold strips a string down to what a person would say aloud.
func fold(s string) string {
	return notAlnum.ReplaceAllString(strings.ToLower(s), "")
}

// sameHost compares two addresses, with or without the usual port.
func sameHost(a, b string) bool {
	trim := func(h string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ":16021") }
	return a != "" && trim(a) == trim(b)
}
