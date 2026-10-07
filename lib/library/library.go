// Package library keeps scenes on disk, apart from any controller: the ones
// being made or changed in the editor, and copies kept of a controller's.
// Each is one file under the root, in the controller's own format, exactly
// what taproot scene dump writes and taproot scene push takes, so the page
// and the command line share them.
package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/assets"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Entry is one scene in the library.
type Entry struct {
	Name     string        `json:"name"`
	File     string        `json:"file"`
	Modified time.Time     `json:"modified"`
	Effect   aurora.Effect `json:"-"`
	// BuiltIn says the scene ships with taproot (assets/scenes) and is not a
	// file in the library's directory; one there of the same name stands in
	// for it.
	BuiltIn bool `json:"builtIn,omitempty"`
}

// ErrNoSuchScene is a name the library does not hold.
var ErrNoSuchScene = errors.New("the library has no scene of that name")

// ErrBuiltIn is a built-in scene being deleted: it is in the binary, not the directory.
var ErrBuiltIn = errors.New("the scene is built into taproot and cannot be deleted; a library scene of the same name stands in for it")

// List reads every scene in the library, by name: the ones built into
// taproot, and the files in the root, a file standing in for a built-in
// scene of the same name. A root that does not exist yet holds only the
// built-in ones. A file that is not a scene is skipped: the library is a
// directory a person may put things in.
func List(root string) ([]Entry, error) {
	out, err := builtIn()
	if err != nil {
		return nil, err
	}

	files, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".json") {
			continue
		}
		path := filepath.Join(root, f.Name())
		e, err := readScene(path)
		if err != nil || e.Name() == "" {
			continue
		}
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		entry := Entry{Name: e.Name(), File: path, Modified: info.ModTime().UTC(), Effect: e}
		if i := slices.IndexFunc(out, func(b Entry) bool { return b.BuiltIn && b.Name == entry.Name }); i >= 0 {
			out[i] = entry
		} else {
			out = append(out, entry)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })

	return out, nil
}

// builtIn is the scenes that ship with taproot.
func builtIn() ([]Entry, error) {
	var out []Entry
	err := fs.WalkDir(assets.Scenes, "scenes", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(assets.Scenes, path)
		if err != nil {
			return err
		}
		var e aurora.Effect
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("built-in scene %s: %w", path, err)
		}
		out = append(out, Entry{Name: e.Name(), File: "taproot:" + path, Effect: e, BuiltIn: true})
		return nil
	})

	return out, err
}

// Read is one scene by name.
func Read(root, name string) (Entry, error) {
	entries, err := List(root)
	if err != nil {
		return Entry{}, err
	}
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Name == name })
	if i < 0 {
		return Entry{}, fmt.Errorf("%w: %q", ErrNoSuchScene, name)
	}

	return entries[i], nil
}

// Write keeps a scene, under its own name: over the file that held a scene
// of that name before, or in a new one. It returns the file.
func Write(root string, e aurora.Effect) (string, error) {
	if e.Name() == "" {
		return "", errors.New("a scene needs a name")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", err
	}
	entries, err := List(root)
	if err != nil {
		return "", err
	}

	path := ""
	taken := map[string]bool{}
	for _, entry := range entries {
		if entry.BuiltIn {
			continue // in the binary, not the directory: a file of its name is the one that stands in for it
		}
		taken[strings.ToLower(filepath.Base(entry.File))] = true
		if entry.Name == e.Name() {
			path = entry.File
		}
	}
	if path == "" {
		path = filepath.Join(root, fileName(e.Name(), taken))
	}

	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return "", err
	}
	pretty.WriteByte('\n')
	if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil { //nolint:gosec // a scene is nothing secret
		return "", err
	}

	return path, nil
}

// Delete removes a scene by name: the library's file, which for a name that
// is also built in leaves the built-in one showing again.
func Delete(root, name string) error {
	entry, err := Read(root, name)
	if err != nil {
		return err
	}
	if entry.BuiltIn {
		return ErrBuiltIn
	}

	return os.Remove(entry.File)
}

func readScene(path string) (aurora.Effect, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the library's own files
	if err != nil {
		return aurora.Effect{}, err
	}
	var e aurora.Effect
	if err := json.Unmarshal(raw, &e); err != nil {
		return aurora.Effect{}, err
	}

	return e, nil
}

// fileName is a scene's file: its own name where a file system takes it,
// with the characters one would not swapped out, and a number where two
// scenes would otherwise land on the same file.
func fileName(scene string, taken map[string]bool) string {
	base := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < ' ' {
			return '_'
		}
		return r
	}, scene)
	if base == "" || base == "." || base == ".." {
		base = "_"
	}

	name := base + ".json"
	for n := 2; taken[strings.ToLower(name)]; n++ {
		name = fmt.Sprintf("%s (%d).json", base, n)
	}

	return name
}
