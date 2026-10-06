package acceptance

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/cli/scene"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Everything here writes to a controller, so it runs only against one that
// has been named as safe to write to (TAPROOT_TEST_SPARE), or against a
// recording of one. The controller the scenes come from is still only read.
//
// What these add is named "taproot test ...", so it cannot be mistaken for a
// scene somebody made, and each test takes away what it added and starts
// again whatever was running before it.

const testScene = "taproot test scene"

// tidy takes a scene the test added off the spare again, and puts back what
// was running, when the test ends however it ends.
func tidy(t *testing.T, c *aurora.Client, names ...string) {
	t.Helper()

	// a recording replays what was recorded; only a real controller has anything to tidy
	before, err := c.SelectedEffect(t.Context())
	if err != nil {
		t.Fatalf("reading what the spare is running: %v", err)
	}
	t.Cleanup(func() {
		// the test's own context has ended by the time this runs
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, name := range names {
			if err := c.DeleteEffect(ctx, name); err != nil && !aurora.IsNotFound(err) {
				t.Errorf("taking %q off the spare again: %v", name, err)
			}
		}
		if !strings.HasPrefix(before, "*") {
			if err := c.SelectEffect(ctx, before); err != nil {
				t.Errorf("starting %q on the spare again: %v", before, err)
			}
		}
	})
}

// colourScene picks a scene of the source's that moves by itself: every
// controller has the plugins those run.
func colourScene(t *testing.T, r *rig) string {
	t.Helper()

	var listed []scene.Listed
	decode(t, r.ok("scene", "list", "source", "--json"), &listed)
	i := slices.IndexFunc(listed, func(s scene.Listed) bool { return s.Kind == aurora.PluginTypeColor })
	if i < 0 {
		t.Skip("the source holds no colour scene to copy")
	}

	return listed[i].Name
}

// The reason taproot exists: a scene on one controller, copied to another
// that does not have it, arrives as it was and plays.
func TestCopyASceneToAnotherController(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.writer("spare") // first: without a spare, or a recording of one, the test is skipped
	r.reader("source")
	spare := r.client("spare")
	tidy(t, spare, testScene)
	name := colourScene(t, r)

	var reports []push.Report
	decode(t, r.ok("scene", "copy", name, "--from", "source", "--to", "spare", "--as", testScene, "--json"), &reports)
	if len(reports) != 1 || len(reports[0].Results) != 1 {
		t.Fatalf("report: %+v", reports)
	}
	res := reports[0].Results[0]
	if res.Scene != testScene || res.Outcome != push.Added {
		t.Fatalf("the copy: %+v", res)
	}
	// a controller on other firmware may store a scene its own way: that is worth knowing, and is not a failure
	if len(res.ReadsBack) > 0 {
		t.Logf("the spare stores the scene its own way: it reads back differently in %v", res.ReadsBack)
	}

	// the spare was backed up before it was written to, without the scene
	b, err := backup.Read(reports[0].Backup)
	if err != nil {
		t.Fatalf("the backup taken first: %v", err)
	}
	if slices.ContainsFunc(b.Scenes, func(e aurora.Effect) bool { return e.Name() == testScene }) {
		t.Error("the backup was taken after the write, not before")
	}

	// it is there, with the colours and the motion it had
	source, err := aurora.ParseEffect([]byte(r.ok("scene", "dump", "source", name)))
	if err != nil {
		t.Fatal(err)
	}
	arrived, err := aurora.ParseEffect([]byte(r.ok("scene", "dump", "spare", testScene)))
	if err != nil {
		t.Fatal(err)
	}
	if arrived.PluginUUID() != source.PluginUUID() || !slices.Equal(arrived.Palette(), source.Palette()) {
		t.Errorf("the scene arrived changed:\n source %v %s\n spare  %v %s", source.Palette(), source.PluginUUID(), arrived.Palette(), arrived.PluginUUID())
	}
	if len(arrived.PluginOptions()) != len(source.PluginOptions()) {
		t.Errorf("the scene's options arrived changed: %v, was %v", arrived.PluginOptions(), source.PluginOptions())
	}
	want(t, r.ok("scene", "list", "spare"), testScene)

	// copied again, there is nothing to do, and nothing is written
	want(t, r.ok("scene", "copy", name, "--from", "source", "--to", "spare", "--as", testScene), "nothing to write", "unchanged")

	// and it plays
	want(t, r.ok("scene", "select", "spare", testScene), "is now running")
	if running, err := spare.SelectedEffect(t.Context()); err != nil || running != testScene {
		t.Errorf("the spare is running %q: %v", running, err)
	}
}

// A scene already on a controller under the same name, and different, is
// never replaced without being told to; told to, it is, and what was there
// is in the backup.
func TestASceneIsOnlyReplacedWhenToldTo(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.writer("spare") // first: without a spare, or a recording of one, the test is skipped
	r.reader("source")
	spare := r.client("spare")
	tidy(t, spare, testScene)
	name := colourScene(t, r)
	dir := t.TempDir()

	// the scene, and the same scene with one colour changed, as files
	original := filepath.Join(dir, "original.json")
	r.ok("scene", "dump", "source", name, "--out", original)
	source, err := aurora.ParseEffect([]byte(r.ok("scene", "dump", "source", name)))
	if err != nil {
		t.Fatal(err)
	}
	palette := source.Palette()
	if len(palette) == 0 {
		t.Skip("the scene has no palette to change")
	}
	palette[0].Hue = float64((int(palette[0].Hue) + 180) % 360)
	altered, err := source.With(aurora.FieldPalette, palette)
	if err != nil {
		t.Fatal(err)
	}
	if err := spare.AddEffect(t.Context(), altered.WithName(testScene)); err != nil {
		t.Fatalf("putting the altered scene on the spare: %v", err)
	}

	// pushed over it: refused, and the spare still holds the altered one
	want(t, r.fails("scene", "push", "spare", original, "--as", testScene), "refused", "a different scene of that name is already there", "palette", "--force")
	held, err := spare.Effect(t.Context(), testScene)
	if err != nil || !slices.Equal(held.Palette(), palette) {
		t.Errorf("a refused push changed the scene: %v, %v", held.Palette(), err)
	}

	// told to: replaced, and the altered one is in the backup taken first
	var reports []push.Report
	decode(t, r.ok("scene", "push", "spare", original, "--as", testScene, "--force", "--json"), &reports)
	if len(reports) != 1 || reports[0].Results[0].Outcome != push.Replaced {
		t.Fatalf("report: %+v", reports)
	}
	held, err = spare.Effect(t.Context(), testScene)
	if err != nil || !slices.Equal(held.Palette(), source.Palette()) {
		t.Errorf("the scene was not replaced: %v, %v", held.Palette(), err)
	}
	b, err := backup.Read(reports[0].Backup)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(b.Scenes, func(e aurora.Effect) bool { return e.Name() == testScene })
	if i < 0 || !slices.Equal(b.Scenes[i].Palette(), palette) {
		t.Error("what was replaced is not in the backup as it was")
	}
}

// A backup of one controller restored onto another adds what the other
// lacks and leaves the rest of it alone.
func TestRestoreOntoAnotherController(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.writer("spare") // first: without a spare, or a recording of one, the test is skipped
	r.reader("source")
	spare := r.client("spare")
	const first, second = "taproot test one", "taproot test two"
	tidy(t, spare, first, second)
	name := colourScene(t, r)

	// a backup holding two scenes the spare does not have, made up from one of the source's
	source, err := aurora.ParseEffect([]byte(r.ok("scene", "dump", "source", name)))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "backup")
	for file, e := range map[string]aurora.Effect{"one.json": source.WithName(first), "two.json": source.WithName(second)} {
		writeScene(t, filepath.Join(dir, backup.ScenesDir, file), e)
	}
	before, err := spare.EffectNames(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	out := r.ok("restore", "spare", dir)
	want(t, out, "backed up to", "added      "+first, "added      "+second)
	after, err := spare.EffectNames(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+2 || !slices.Contains(after, first) || !slices.Contains(after, second) {
		t.Errorf("the spare went from %d scenes to %d: %v", len(before), len(after), after)
	}
	for _, kept := range before {
		if !slices.Contains(after, kept) {
			t.Errorf("the restore lost %q", kept)
		}
	}

	want(t, r.ok("restore", "spare", dir), "nothing to write")
}
