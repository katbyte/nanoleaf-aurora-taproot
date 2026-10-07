// Package backup saves everything a controller holds to a directory and reads
// it back: every scene as its own file, exactly as the controller sent it,
// with what the controller says about itself beside them.
//
// A backup is never written over. Its files are made read-only, and taking
// one into a directory that already holds something is refused.
package backup

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// The files of a backup.
const (
	ManifestFile  = "manifest.json"
	ScenesDir     = "scenes"
	AllScenesFile = "all-scenes.json"
	InfoFile      = "info.json"
	PluginsFile   = "plugins.json"

	stampLayout = "20060102-150405"
)

// Manifest says what a backup holds and what it was taken from.
type Manifest struct {
	Controller Source          `json:"controller"`
	Taken      string          `json:"taken"`
	Running    string          `json:"running"`
	Scenes     []Scene         `json:"scenes"`
	Files      map[string]File `json:"files"`
}

// Source is the controller a backup was taken from.
type Source struct {
	Name     string `json:"name"`
	Host     string `json:"ip"`
	Model    string `json:"model"`
	Serial   string `json:"serialNo"`
	Firmware string `json:"firmwareVersion"`
}

// Scene is one scene of a backup.
type Scene struct {
	Name       string `json:"name"`
	File       string `json:"file"`
	Type       string `json:"animType"`
	PluginType string `json:"pluginType"`
	PluginUUID string `json:"pluginUuid"`
	Colours    int    `json:"colours"`
	Bytes      int    `json:"bytes"`
}

// File is the size and checksum of one file of a backup.
type File struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Backup is a backup read back from disk.
type Backup struct {
	Dir string
	// Manifest is nil for a directory of scene files with no manifest, which
	// is still a backup: the scenes are what matter.
	Manifest *Manifest
	Scenes   []aurora.Effect
}

// Dir is where a backup of a controller taken at a time goes under root: a
// directory named for the second, or, when two are taken within one, as a
// script copying several scenes in a row will, the first of name-2, name-3
// and so on that is not there yet.
func Dir(root, controller string, at time.Time) string {
	base := filepath.Join(root, controller, at.Format(stampLayout))
	dir := base
	for n := 2; n < 1000; n++ {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			break
		}
		dir = fmt.Sprintf("%s-%d", base, n)
	}

	return dir
}

// Take reads everything from the controller and writes it into dir, which
// must not hold anything yet. Each scene is asked for by itself and checked
// against the controller's answer for all of them at once, so a scene that
// reads two ways is caught here, while the controller still has it, rather
// than at a restore.
func Take(ctx context.Context, c *aurora.Client, dir string) (*Manifest, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s already holds files: a backup is never written over", dir)
	}

	info, err := c.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the controller: %w", err)
	}
	all, err := c.Effects(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the scenes: %w", err)
	}
	plugins, err := c.Plugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the plugins: %w", err)
	}

	// read everything before writing anything: a controller that drops off part way leaves no half backup
	scenes, err := readEach(ctx, c, info.Effects.List, all)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Join(dir, ScenesDir), 0o700); err != nil {
		return nil, fmt.Errorf("creating the backup directory: %w", err)
	}

	m := &Manifest{
		Controller: Source{Name: info.Name, Host: c.Host(), Model: info.Model, Serial: info.SerialNo, Firmware: info.FirmwareVersion},
		Taken:      time.Now().Format(time.RFC3339),
		Running:    info.Effects.Select,
		Files:      map[string]File{},
	}
	if _, err := m.save(dir, InfoFile, info.Raw); err != nil {
		return nil, err
	}
	if _, err := m.save(dir, PluginsFile, map[string]any{"plugins": plugins}); err != nil {
		return nil, err
	}
	if _, err := m.save(dir, AllScenesFile, map[string]any{"animations": all}); err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	for _, e := range scenes {
		rel := filepath.Join(ScenesDir, fileName(e.Name(), taken))
		n, serr := m.save(dir, rel, e)
		if serr != nil {
			return nil, serr
		}
		m.Scenes = append(m.Scenes, Scene{
			Name: e.Name(), File: filepath.ToSlash(rel), Type: e.Type(), PluginType: e.PluginType(),
			PluginUUID: e.PluginUUID(), Colours: len(e.Palette()), Bytes: n,
		})
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), append(data, '\n'), 0o400); err != nil {
		return nil, fmt.Errorf("writing the backup: %w", err)
	}

	return m, nil
}

// readEach asks for each scene by itself and checks it against the answer for
// all of them at once.
func readEach(ctx context.Context, c *aurora.Client, names []string, all []aurora.Effect) ([]aurora.Effect, error) {
	scenes := make([]aurora.Effect, 0, len(names))
	for _, name := range names {
		e, err := c.Effect(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("reading scene %q: %w", name, err)
		}
		i := slices.IndexFunc(all, func(x aurora.Effect) bool { return x.Name() == name })
		if i < 0 || !all[i].Equal(e) {
			return nil, fmt.Errorf("scene %q reads differently by itself than among all of them: not saving a backup that cannot be trusted", name)
		}
		scenes = append(scenes, e)
	}

	return scenes, nil
}

// save writes one file of a backup, read-only, and notes its size and
// checksum in the manifest.
func (m *Manifest) save(dir, rel string, v any) (int, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", rel, err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), data, 0o400); err != nil {
		return 0, fmt.Errorf("writing the backup: %w", err)
	}
	sum := sha256.Sum256(data)
	m.Files[filepath.ToSlash(rel)] = File{Bytes: len(data), SHA256: hex.EncodeToString(sum[:])}

	return len(data), nil
}

// fileName is a scene's file: its own name where a file system takes it, with
// the characters one would not swapped out, and a number where two scenes
// would otherwise land on the same file.
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

	name := base
	for n := 2; taken[strings.ToLower(name)]; n++ { // some file systems fold case
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	taken[strings.ToLower(name)] = true

	return name + ".json"
}

// Read reads a backup from dir and checks it: every file the manifest lists
// must be there with the checksum it had when it was written. A directory of
// scene files with no manifest is read as it is.
func Read(dir string) (*Backup, error) {
	b := &Backup{Dir: dir}

	data, err := os.ReadFile(filepath.Join(dir, ManifestFile)) //nolint:gosec // the backup the person named
	switch {
	case err == nil:
		b.Manifest = &Manifest{}
		if err := json.Unmarshal(data, b.Manifest); err != nil {
			return nil, fmt.Errorf("%s: the manifest is not valid: %w", dir, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("reading the backup: %w", err)
	}

	var files []string
	if b.Manifest != nil {
		if err := b.Manifest.check(dir); err != nil {
			return nil, err
		}
		for _, s := range b.Manifest.Scenes {
			files = append(files, filepath.Join(dir, filepath.FromSlash(s.File)))
		}
	} else {
		files, err = filepath.Glob(filepath.Join(dir, ScenesDir, "*.json"))
		if err != nil || len(files) == 0 {
			return nil, fmt.Errorf("%s is not a backup: no %s and no %s/*.json", dir, ManifestFile, ScenesDir)
		}
		slices.Sort(files)
	}

	for _, f := range files {
		e, err := readScene(f)
		if err != nil {
			return nil, err
		}
		b.Scenes = append(b.Scenes, e)
	}

	return b, nil
}

// check reads every file the manifest lists and compares it with the
// checksum it had when the backup was taken.
func (m *Manifest) check(dir string) error {
	for rel, want := range m.Files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // inside the backup the person named
		if err != nil {
			return fmt.Errorf("the backup is missing %s: %w", rel, err)
		}
		if sum := sha256.Sum256(got); hex.EncodeToString(sum[:]) != want.SHA256 {
			return fmt.Errorf("%s has changed since the backup was taken: its checksum does not match the manifest", rel)
		}
	}

	return nil
}

func readScene(path string) (aurora.Effect, error) {
	data, err := os.ReadFile(path) //nolint:gosec // inside the backup the person named
	if err != nil {
		return aurora.Effect{}, fmt.Errorf("reading the backup: %w", err)
	}
	e, err := aurora.ParseEffect(data)
	if err != nil {
		return aurora.Effect{}, fmt.Errorf("%s: %w", path, err)
	}
	if e.Name() == "" {
		return aurora.Effect{}, fmt.Errorf("%s: a scene with no name", path)
	}

	return e, nil
}

// Entry is one backup found under a root.
type Entry struct {
	Controller string    `json:"controller"`
	Dir        string    `json:"dir"`
	Taken      time.Time `json:"taken"`
	Scenes     int       `json:"scenes"`
}

// List finds the backups under root, newest first: every directory two levels
// down that holds a manifest.
func List(root string) ([]Entry, error) {
	manifests, err := filepath.Glob(filepath.Join(root, "*", "*", ManifestFile))
	if err != nil {
		return nil, err
	}

	out := make([]Entry, 0, len(manifests))
	for _, path := range manifests {
		dir := filepath.Dir(path)
		data, err := os.ReadFile(path) //nolint:gosec // a manifest under the backups directory
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		taken, _ := time.Parse(time.RFC3339, m.Taken) // a backup with no readable time sorts last
		if taken.IsZero() {
			// backups taken before taproot wrote them carry the time without the colon in its zone
			taken, _ = time.Parse("2006-01-02T15:04:05-0700", m.Taken) // as above
		}
		out = append(out, Entry{Controller: filepath.Base(filepath.Dir(dir)), Dir: dir, Taken: taken, Scenes: len(m.Scenes)})
	}
	// two taken within a second of each other are told apart by their directories, the later of which has the higher number
	slices.SortFunc(out, func(a, b Entry) int {
		return cmp.Or(b.Taken.Compare(a.Taken), strings.Compare(b.Dir, a.Dir))
	})

	return out, nil
}
