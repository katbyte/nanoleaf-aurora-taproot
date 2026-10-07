package library_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/library"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

func TestLibrary(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "scenes") // not there yet: only what ships with taproot
	shipped, err := library.List(root)
	if err != nil || len(shipped) == 0 || !shipped[0].BuiltIn || shipped[0].Name != "kt Northern Lights" {
		t.Fatalf("the built-in scenes: %+v, %v", shipped, err)
	}
	builtIns := len(shipped)
	// a built-in scene cannot be deleted; a file of its name stands in for it, and deleting that shows it again
	if err := library.Delete(root, "kt Northern Lights"); !errors.Is(err, library.ErrBuiltIn) {
		t.Errorf("deleting a built-in scene: %v", err)
	}
	mine, _ := shipped[0].Effect.With("palette", []aurora.Color{{Hue: 1}})
	if _, err := library.Write(root, mine); err != nil {
		t.Fatal(err)
	}
	if entry, err := library.Read(root, "kt Northern Lights"); err != nil || entry.BuiltIn || !entry.Effect.Equal(mine) {
		t.Errorf("the file should stand in for the built-in one: %+v, %v", entry, err)
	}
	if err := library.Delete(root, "kt Northern Lights"); err != nil {
		t.Fatal(err)
	}
	if entry, err := library.Read(root, "kt Northern Lights"); err != nil || !entry.BuiltIn {
		t.Errorf("the built-in one should show again: %+v, %v", entry, err)
	}

	ctl := auroratest.New(t)
	flames, _ := ctl.Effect("Flames")
	forest, _ := ctl.Effect("Forest")

	// kept under its own name, in the controller's format: what scene push takes
	path, err := library.Write(root, flames)
	if err != nil || filepath.Base(path) != "Flames.json" {
		t.Fatalf("write: %q, %v", path, err)
	}
	if _, err := library.Write(root, forest); err != nil {
		t.Fatal(err)
	}
	entries, err := library.List(root)
	if err != nil || len(entries) != builtIns+2 || entries[0].Name != "Flames" || entries[1].Name != "Forest" {
		t.Fatalf("list: %+v, %v", entries, err)
	}
	if !entries[0].Effect.Equal(flames) {
		t.Error("Flames came back changed")
	}

	// a change to a scene goes over the same file
	warmer, _ := flames.With("palette", []aurora.Color{{Hue: 30, Saturation: 100, Brightness: 100}})
	if again, err := library.Write(root, warmer); err != nil || again != path {
		t.Errorf("writing it again: %q, %v", again, err)
	}
	if entry, err := library.Read(root, "Flames"); err != nil || !entry.Effect.Equal(warmer) {
		t.Errorf("read after the change: %+v, %v", entry, err)
	}

	// a name a file system would not take is still a scene
	if p, err := library.Write(root, flames.WithName("a/b: c")); err != nil || filepath.Base(p) != "a_b_ c.json" {
		t.Errorf("an odd name: %q, %v", p, err)
	}
	// something that is not a scene in the directory is left alone and not listed
	if err := os.WriteFile(filepath.Join(root, "notes.json"), []byte(`{"hello": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if entries, _ := library.List(root); len(entries) != builtIns+3 {
		t.Errorf("listed: %+v", entries)
	}

	if err := library.Delete(root, "Forest"); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Read(root, "Forest"); !errors.Is(err, library.ErrNoSuchScene) {
		t.Errorf("after deleting: %v", err)
	}
	if _, err := library.Write(root, aurora.Effect{}); err == nil {
		t.Error("a scene with no name was kept")
	}
}
