package backup_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const scene = "kt Northern Lights"

func controller(t *testing.T) (*auroratest.Controller, *aurora.Client) {
	t.Helper()

	ctl := auroratest.New(t)
	c, err := aurora.New(ctl.Host(), ctl.Token())
	if err != nil {
		t.Fatal(err)
	}

	return ctl, c
}

func TestTakeAndRead(t *testing.T) {
	t.Parallel()

	ctl, c := controller(t)
	dir := filepath.Join(t.TempDir(), "office", "20261006-120000")

	m, err := backup.Take(t.Context(), c, dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Controller.Name != "Light Panels 53:A6:3C" || m.Controller.Model != "NL22" || m.Controller.Firmware != "5.2.1" || m.Controller.Host != ctl.Host() {
		t.Errorf("what it was taken from: %+v", m.Controller)
	}
	if m.Running != scene || len(m.Scenes) != 17 {
		t.Errorf("running %q, %d scenes", m.Running, len(m.Scenes))
	}
	if _, err := time.Parse(time.RFC3339, m.Taken); err != nil {
		t.Errorf("taken at %q: %v", m.Taken, err)
	}
	last := m.Scenes[16]
	if last.Name != scene || last.File != "scenes/kt Northern Lights.json" || last.Type != "plugin" || last.PluginType != "color" || last.Colours != 7 || last.Bytes == 0 {
		t.Errorf("the scene's entry: %+v", last)
	}
	// the manifest lists every file but itself
	if len(m.Files) != 20 {
		t.Errorf("%d files listed, want 17 scenes and 3 others", len(m.Files))
	}

	// a scene's file is the scene, byte for byte as the controller holds it
	held, _ := ctl.Effect(scene)
	want, _ := json.Marshal(held)
	got, err := os.ReadFile(filepath.Join(dir, "scenes", "kt Northern Lights.json")) //nolint:gosec // the backup this test took
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("the scene's file:\n got %s\nwant %s (%v)", got, want, err)
	}

	// taking a backup only reads
	if w := ctl.Writes(); len(w) != 0 {
		t.Errorf("a backup wrote to the controller: %+v", w)
	}

	if runtime.GOOS != "windows" {
		if info, serr := os.Stat(filepath.Join(dir, "scenes", "kt Northern Lights.json")); serr != nil || info.Mode().Perm() != 0o400 {
			t.Errorf("a backup's files are read-only: %v", serr)
		}
	}

	b, err := backup.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Dir != dir || b.Manifest == nil || b.Manifest.Running != scene || len(b.Scenes) != 17 {
		t.Fatalf("read back: %+v", b)
	}
	for _, e := range b.Scenes {
		if on, ok := ctl.Effect(e.Name()); !ok || !on.Equal(e) {
			t.Errorf("%q came back from the backup changed", e.Name())
		}
	}
}

func TestTakeNeverWritesOverABackup(t *testing.T) {
	t.Parallel()

	_, c := controller(t)
	dir := t.TempDir()
	if _, err := backup.Take(t.Context(), c, dir); err != nil {
		t.Fatalf("an empty directory is fine: %v", err)
	}
	if _, err := backup.Take(t.Context(), c, dir); err == nil || !strings.Contains(err.Error(), "never written over") {
		t.Errorf("a second backup into the same directory: %v", err)
	}
}

// A controller that drops off part way leaves nothing behind: half a backup
// would look like a whole one.
func TestTakeLeavesNothingWhenItFails(t *testing.T) {
	t.Parallel()

	for name, fail := range map[string]func(*auroratest.Controller){
		"the controller itself": func(ctl *auroratest.Controller) { ctl.Fail(http.MethodGet, "/", http.StatusInternalServerError) },
		"all the scenes":        func(ctl *auroratest.Controller) { ctl.FailCommand("requestAll", http.StatusInternalServerError) },
		"the plugins":           func(ctl *auroratest.Controller) { ctl.FailCommand("requestPlugins", http.StatusInternalServerError) },
		"one scene":             func(ctl *auroratest.Controller) { ctl.FailCommand("request", http.StatusInternalServerError) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctl, c := controller(t)
			fail(ctl)
			dir := filepath.Join(t.TempDir(), "backup")
			if _, err := backup.Take(t.Context(), c, dir); err == nil {
				t.Fatal("want an error")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("the directory was created: %v", err)
			}
		})
	}
}

func TestReadChecksTheBackup(t *testing.T) {
	t.Parallel()

	take := func(t *testing.T) string {
		t.Helper()
		_, c := controller(t)
		dir := filepath.Join(t.TempDir(), "backup")
		if _, err := backup.Take(t.Context(), c, dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	file := func(dir string) string { return filepath.Join(dir, "scenes", "Flames.json") }

	t.Run("a scene edited since", func(t *testing.T) {
		t.Parallel()
		dir := take(t)
		if err := os.Chmod(file(dir), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file(dir), []byte(`{"animName":"Flames"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := backup.Read(dir); err == nil || !strings.Contains(err.Error(), "scenes/Flames.json has changed") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a scene gone", func(t *testing.T) {
		t.Parallel()
		dir := take(t)
		if err := os.Remove(file(dir)); err != nil {
			t.Fatal(err)
		}
		if _, err := backup.Read(dir); err == nil || !strings.Contains(err.Error(), "missing scenes/Flames.json") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a manifest that is not one", func(t *testing.T) {
		t.Parallel()
		dir := take(t)
		path := filepath.Join(dir, backup.ManifestFile)
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := backup.Read(dir); err == nil || !strings.Contains(err.Error(), "manifest is not valid") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("not a backup at all", func(t *testing.T) {
		t.Parallel()
		if _, err := backup.Read(t.TempDir()); err == nil || !strings.Contains(err.Error(), "is not a backup") {
			t.Errorf("got %v", err)
		}
	})
}

// A directory of scene files, as somebody might put together by hand from
// dumps, restores without a manifest.
func TestReadWithoutAManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scenes := filepath.Join(dir, backup.ScenesDir)
	if err := os.Mkdir(scenes, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string]string{"b.json": `{"animName":"B"}`, "a.json": `{"animName":"A","palette":[]}`} {
		if err := os.WriteFile(filepath.Join(scenes, name), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	b, err := backup.Read(dir)
	if err != nil || b.Manifest != nil || len(b.Scenes) != 2 || b.Scenes[0].Name() != "A" || b.Scenes[1].Name() != "B" {
		t.Fatalf("read: %+v, %v", b, err)
	}

	// a file among them that is not a scene is an error, not a scene skipped in silence
	for name, doc := range map[string]string{"c.json": `{"palette":[]}`, "d.json": `[1]`} {
		bad := t.TempDir()
		if err := os.Mkdir(filepath.Join(bad, backup.ScenesDir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bad, backup.ScenesDir, name), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := backup.Read(bad); err == nil {
			t.Errorf("%s: want an error for %s", name, doc)
		}
	}
}

// Scenes whose names a file system would not take, or would take as one
// another, each still get a file of their own.
func TestSceneNamesThatAreNotFileNames(t *testing.T) {
	t.Parallel()

	ctl, c := controller(t)
	names := []string{"a/b", "A/B", `c:\d`, "..", "tab\there"}
	for _, name := range names {
		e, _ := ctl.Effect("Flames")
		ctl.PutEffect(e.WithName(name))
	}

	dir := filepath.Join(t.TempDir(), "backup")
	m, err := backup.Take(t.Context(), c, dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, s := range m.Scenes {
		files[s.Name] = s.File
	}
	for name, want := range map[string]string{
		"a/b":       "scenes/a_b (2).json", // capitals sort first, so A/B is the one that got the plain name
		"A/B":       "scenes/A_B.json",
		`c:\d`:      "scenes/c__d.json",
		"..":        "scenes/_.json",
		"tab\there": "scenes/tab_here.json",
	} {
		if files[name] != want {
			t.Errorf("%q went to %q, want %q", name, files[name], want)
		}
	}

	b, err := backup.Read(dir)
	if err != nil || len(b.Scenes) != 22 {
		t.Fatalf("read back %d scenes: %v", len(b.Scenes), err)
	}
	for _, name := range names {
		if !slices.ContainsFunc(b.Scenes, func(e aurora.Effect) bool { return e.Name() == name }) {
			t.Errorf("%q did not come back", name)
		}
	}
}

func TestDirAndList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	at := time.Date(2026, 10, 6, 12, 36, 29, 0, time.UTC)
	if got, want := backup.Dir(root, "office", at), filepath.Join(root, "office", "20261006-123629"); got != want {
		t.Errorf("Dir: %s, want %s", got, want)
	}

	if list, err := backup.List(root); err != nil || len(list) != 0 {
		t.Errorf("nothing yet: %+v, %v", list, err)
	}

	_, c := controller(t)
	older := backup.Dir(root, "office", at)
	if _, err := backup.Take(t.Context(), c, older); err != nil {
		t.Fatal(err)
	}
	// one taken before taproot wrote them, with the time as a script would print it, and one that cannot be read
	byHand := filepath.Join(root, "bedroom_10.0.5.182", "20200101-000000")
	broken := filepath.Join(root, "bedroom_10.0.5.182", "broken")
	for dir, manifest := range map[string]string{
		byHand: `{"taken":"2020-01-01T00:00:00-0800","scenes":[{"name":"A"},{"name":"B"}]}`,
		broken: `{`,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, backup.ManifestFile), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	list, err := backup.List(root)
	if err != nil || len(list) != 2 {
		t.Fatalf("List: %+v, %v", list, err)
	}
	// newest first
	if list[0].Controller != "office" || list[0].Dir != older || list[0].Scenes != 17 || list[0].Taken.IsZero() {
		t.Errorf("the newer: %+v", list[0])
	}
	if list[1].Controller != "bedroom_10.0.5.182" || list[1].Scenes != 2 || list[1].Taken.Year() != 2020 {
		t.Errorf("the older: %+v", list[1])
	}
}
