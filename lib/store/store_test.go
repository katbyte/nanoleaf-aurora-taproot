package store

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func three() *Store {
	return &Store{Controllers: []Controller{
		{Name: "office", Device: "Light Panels 53:A6:3C", Host: "10.0.5.183", Serial: "S1", Token: "t1"},
		{Name: "bedroom", Device: "Light Panels 52:56:C3", Host: "10.0.5.182", Serial: "S2", Token: "t2"},
		{Name: "bed", Device: "Light Panels 51:1D:7D", Host: "panels.lan:8080", Serial: "S3", Token: "t3"},
	}}
}

func TestOpenAndSave(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "taproot")
	s, err := Open(dir)
	if err != nil || len(s.Controllers) != 0 {
		t.Fatalf("a directory that is not there yet is an empty store: %+v, %v", s, err)
	}
	if s.Path() != filepath.Join(dir, File) {
		t.Errorf("Path: %s", s.Path())
	}

	added := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s.Put(Controller{Name: "office", Device: "Light Panels 53:A6:3C", Host: "10.0.5.183", Serial: "S1", Model: "NL22", Firmware: "5.2.1", Token: "secret", Added: added})
	s.Put(Controller{Name: "bedroom", Host: "10.0.5.182", Token: "other"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	back, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// saved by name, so the file reads the same whatever order they were connected in
	if got := back.Names(); !slices.Equal(got, []string{"bedroom", "office"}) {
		t.Errorf("names: %v", got)
	}
	office, err := back.Find("office")
	if err != nil || office.Token != "secret" || office.Firmware != "5.2.1" || !office.Added.Equal(added) {
		t.Errorf("office came back as %+v, %v", office, err)
	}

	// nothing is left behind by the save but the file itself
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != File {
		t.Errorf("the directory holds %v", entries)
	}
}

// The file holds tokens that never expire: only its owner may read it.
func TestOnlyTheOwnerCanReadIt(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("windows has no permission bits to check")
	}

	dir := filepath.Join(t.TempDir(), "taproot")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Put(Controller{Name: "office", Host: "10.0.5.183", Token: "secret"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	mode := func(path string) os.FileMode {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	if m := mode(s.Path()); m != 0o600 {
		t.Errorf("the file is %o, want 600", m)
	}
	if m := mode(dir); m != 0o700 {
		t.Errorf("the directory is %o, want 700", m)
	}

	// a file that came back from a copy readable by everyone is closed again on the next read
	if err := os.Chmod(s.Path(), 0o644); err != nil { //nolint:gosec // loosening it is the test
		t.Fatal(err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if m := mode(s.Path()); m != 0o600 {
		t.Errorf("after reading a loose file it is %o, want 600", m)
	}
}

func TestOpenRefusesAFileItCannotRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("want an error naming the file as not valid, got %v", err)
	}

	// a directory where the file should be
	odd := t.TempDir()
	if err := os.Mkdir(filepath.Join(odd, File), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(odd); err == nil {
		t.Error("want an error for a file that cannot be read")
	}
	s := &Store{path: filepath.Join(odd, File)}
	if err := s.Save(); err == nil {
		t.Error("want an error saving over a directory")
	}
}

func TestPut(t *testing.T) {
	t.Parallel()

	s := three()

	// the same device, by its serial number, is replaced even at a new address and under a new name
	name, replaced := s.Put(Controller{Name: "study", Device: "Light Panels 53:A6:3C", Host: "10.0.5.200", Serial: "S1", Token: "new"})
	if name != "study" || !replaced || len(s.Controllers) != 3 {
		t.Errorf("Put a known serial: %q, %v, %d controllers", name, replaced, len(s.Controllers))
	}
	if c, err := s.Find("study"); err != nil || c.Token != "new" || c.Host != "10.0.5.200" {
		t.Errorf("study: %+v, %v", c, err)
	}

	// with no serial to go by, the same address is the same device; connected again with no name given, it keeps its name
	name, replaced = s.Put(Controller{Device: "Light Panels 52:56:C3", Host: "10.0.5.182:16021", Token: "again"})
	if name != "bedroom" || !replaced || len(s.Controllers) != 3 {
		t.Errorf("Put a known address: %q, %v, %d controllers", name, replaced, len(s.Controllers))
	}
	if c, err := s.Find("bedroom"); err != nil || c.Token != "again" {
		t.Errorf("bedroom: %+v, %v", c, err)
	}

	// a new device with no name of its own takes its device's, and one whose name is taken gets a number
	name, replaced = s.Put(Controller{Device: "Light Panels AA:BB:CC", Host: "10.0.5.9", Serial: "S9", Token: "t9"})
	if name != "light-panels-aa-bb-cc" || replaced {
		t.Errorf("Put a new device: %q, %v", name, replaced)
	}
	name, _ = s.Put(Controller{Name: "study", Host: "10.0.5.10", Serial: "S10", Token: "t10"})
	if name != "study-2" {
		t.Errorf("a second study: %q", name)
	}
	name, _ = s.Put(Controller{Name: "study", Host: "10.0.5.11", Serial: "S11", Token: "t11"})
	if name != "study-3" {
		t.Errorf("a third study: %q", name)
	}
	// connecting the first study again keeps its name rather than taking a number
	name, replaced = s.Put(Controller{Name: "study", Host: "10.0.5.200", Serial: "S1", Token: "newer"})
	if name != "study" || !replaced {
		t.Errorf("study again: %q, %v", name, replaced)
	}
	// with neither a name nor a device, the address has to do
	if name, _ = s.Put(Controller{Host: "10.0.5.77", Token: "t"}); name != "10-0-5-77" {
		t.Errorf("a nameless device: %q", name)
	}
}

func TestRename(t *testing.T) {
	t.Parallel()

	s := three()
	// kept to what a command line and a directory both take without quoting
	if name, err := s.Rename("office", " Living Room! "); err != nil || name != "living-room" {
		t.Fatalf("Rename: %q, %v", name, err)
	}
	if c, err := s.Find("living-room"); err != nil || c.Token != "t1" || c.Device != "Light Panels 53:A6:3C" {
		t.Errorf("the renamed controller: %+v, %v", c, err)
	}
	if _, err := s.Find("office"); err == nil {
		t.Error("the old name still finds it")
	}
	// to the name it has: nothing to do, and no error
	if name, err := s.Rename("living-room", "Living-Room"); err != nil || name != "living-room" {
		t.Errorf("to its own name: %q, %v", name, err)
	}

	for _, bad := range []struct{ from, to, msg string }{
		{"living-room", "bedroom", "bedroom is already what Light Panels 52:56:C3 is called"},
		{"living-room", "BED", "bed is already what"},
		{"living-room", " !! ", "at least one letter or digit"},
		{"kitchen", "pantry", `no controller called "kitchen"`},
	} {
		if _, err := s.Rename(bad.from, bad.to); err == nil || !strings.Contains(err.Error(), bad.msg) {
			t.Errorf("Rename(%q, %q): %v; want an error saying %q", bad.from, bad.to, err, bad.msg)
		}
	}
	if got := s.Names(); !slices.Equal(got, []string{"bed", "bedroom", "living-room"}) {
		t.Errorf("after it all: %v", got)
	}

	// a name given when connecting is kept to the same
	if name, _ := s.Put(Controller{Name: "The Hall", Host: "10.0.5.50", Serial: "S50", Token: "t"}); name != "the-hall" {
		t.Errorf("a name given to Put: %q", name)
	}
}

func TestRemove(t *testing.T) {
	t.Parallel()

	s := three()
	if !s.Remove("bedroom") || len(s.Controllers) != 2 {
		t.Error("Remove did not take bedroom out")
	}
	if s.Remove("bedroom") || s.Remove("light panels") {
		t.Error("Remove goes by the exact name only")
	}
}

func TestFind(t *testing.T) {
	t.Parallel()

	s := three()
	for ref, want := range map[string]string{
		"office":                "office",  // its name
		"OFFICE":                "office",  // whatever the case
		"Light Panels 53:A6:3C": "office",  // what it calls itself
		"53a63c":                "office",  // part of that, without the punctuation
		"10.0.5.183":            "office",  // its address
		"183":                   "office",  // part of that
		"S2":                    "bedroom", // its serial number
		"bed":                   "bed",     // a whole name wins over the start of another
		"bedr":                  "bedroom",
		"panels.lan":            "bed",
		"8080":                  "bed",
	} {
		got, err := s.Find(ref)
		if err != nil || got.Name != want {
			t.Errorf("Find(%q) = %q, %v; want %q", ref, got.Name, err, want)
		}
	}

	for ref, want := range map[string]string{
		"light panels": "matches several controllers: office, bedroom, bed",
		"10.0.5":       "matches several",
		"kitchen":      `no controller matches "kitchen": taproot has bed, bedroom, office`,
		"":             "which controller? one of: bed, bedroom, office",
		" :: ":         "which controller?",
	} {
		if _, err := s.Find(ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Find(%q): %v; want an error saying %q", ref, err, want)
		}
	}

	if _, err := (&Store{}).Find("office"); err == nil || !strings.Contains(err.Error(), "taproot connect") {
		t.Errorf("an empty store should say how to fill it: %v", err)
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Light Panels 53:A6:3C": "light-panels-53-a6-3c",
		"  Living Room!  ":      "living-room",
		"10.0.5.183":            "10-0-5-183",
		"---":                   "",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultDir(t *testing.T) {
	t.Parallel()

	if dir := DefaultDir(); !strings.HasSuffix(filepath.ToSlash(dir), ".config/taproot") {
		t.Errorf("DefaultDir: %s", dir)
	}
}
