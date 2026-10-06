package push_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora/auroratest"
)

const scene = "kt Northern Lights"

// rig is a controller to push to, the scene to push (read from another
// controller that has it), and where its backups go.
type rig struct {
	ctl   *auroratest.Controller
	c     *aurora.Client
	scene aurora.Effect
	opt   push.Options
}

func newRig(t *testing.T, opts ...aurora.Option) rig {
	t.Helper()

	source := auroratest.New(t)
	e, ok := source.Effect(scene)
	if !ok {
		t.Fatal("the fixture has lost its scene")
	}

	// the controller being pushed to is one that lost the scene
	ctl := auroratest.New(t)
	ctl.RemoveEffect(scene)
	c, err := aurora.New(ctl.Host(), ctl.Token(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	return rig{ctl: ctl, c: c, scene: e, opt: push.Options{
		Controller: "bedroom", BackupRoot: t.TempDir(), Now: func() time.Time { return at },
	}}
}

func (r rig) push(t *testing.T, scenes ...aurora.Effect) push.Report {
	t.Helper()

	report, err := push.Scenes(t.Context(), r.c, scenes, r.opt)
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	return report
}

func TestAddsASceneThatIsNotThere(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	report := r.push(t, r.scene)

	if len(report.Results) != 1 || report.Results[0].Scene != scene || report.Results[0].Outcome != push.Added {
		t.Fatalf("report: %+v", report)
	}
	if res := report.Results[0]; len(res.ReadsBack) != 0 || res.Reason != "" {
		t.Errorf("it should read back as it was sent: %+v", res)
	}
	if report.Wrote() != 1 || report.Err() != nil || report.Controller != "bedroom" || report.DryRun {
		t.Errorf("wrote %d, err %v: %+v", report.Wrote(), report.Err(), report)
	}

	on, ok := r.ctl.Effect(scene)
	if !ok || !on.Equal(r.scene) {
		t.Error("the scene is not on the controller as it was sent")
	}

	// the controller was backed up before the write, as it was before the write
	if want := filepath.Join(r.opt.BackupRoot, "bedroom", "20261006-120000"); report.Backup != want {
		t.Errorf("backup at %q, want %q", report.Backup, want)
	}
	b, err := backup.Read(report.Backup)
	if err != nil || len(b.Scenes) != 16 {
		t.Fatalf("the backup: %d scenes, %v", len(b.Scenes), err)
	}
	// one write, and it came after every read the backup made
	writes := r.ctl.Writes()
	if len(writes) != 1 || !strings.HasPrefix(writes[0].Body, `{"write":{"command":"add"`) {
		t.Errorf("writes: %+v", writes)
	}
}

func TestLeavesAloneWhatIsAlreadyThere(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.ctl.PutEffect(r.scene)

	report := r.push(t, r.scene)
	if report.Results[0].Outcome != push.Unchanged || report.Wrote() != 0 || report.Err() != nil {
		t.Errorf("report: %+v", report)
	}
	// nothing to write, so nothing written and nothing backed up
	if report.Backup != "" || len(r.ctl.Writes()) != 0 {
		t.Errorf("backup %q, writes %+v", report.Backup, r.ctl.Writes())
	}
	if entries, _ := os.ReadDir(r.opt.BackupRoot); len(entries) != 0 {
		t.Errorf("a backup was taken: %v", entries)
	}
}

func TestRefusesToReplaceWithoutForce(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	// the controller has a scene of that name that somebody has since changed
	mine, err := r.scene.With("palette", []aurora.Color{{Hue: 10, Saturation: 100, Brightness: 100}})
	if err != nil {
		t.Fatal(err)
	}
	r.ctl.PutEffect(mine)

	report := r.push(t, r.scene)
	res := report.Results[0]
	if res.Outcome != push.Refused || len(res.Differs) != 1 || res.Differs[0] != "palette" {
		t.Fatalf("report: %+v", report)
	}
	if !strings.Contains(res.Reason, "differs in palette") || !strings.Contains(res.Reason, "--force") {
		t.Errorf("the reason should say what differs and how to go ahead: %q", res.Reason)
	}
	if err := report.Err(); err == nil || !strings.Contains(err.Error(), `bedroom: "kt Northern Lights" refused`) {
		t.Errorf("Err: %v", err)
	}
	if on, _ := r.ctl.Effect(scene); !on.Equal(mine) || len(r.ctl.Writes()) != 0 || report.Backup != "" {
		t.Error("a refused scene must leave the controller untouched and unbacked-up")
	}

	// with force it is replaced, and what was there is in the backup
	r.opt.Force = true
	report = r.push(t, r.scene)
	if res := report.Results[0]; res.Outcome != push.Replaced || len(res.Differs) != 1 || report.Wrote() != 1 {
		t.Fatalf("forced: %+v", report)
	}
	if on, _ := r.ctl.Effect(scene); !on.Equal(r.scene) {
		t.Error("the scene was not replaced")
	}
	b, err := backup.Read(report.Backup)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range b.Scenes {
		if e.Name() == scene {
			found = e.Equal(mine)
		}
	}
	if !found {
		t.Error("the scene that was replaced is not in the backup as it was")
	}
}

func TestRefusesWhatTheControllerCouldNotPlay(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	foreign, err := r.scene.With(aurora.FieldPluginUUID, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	nameless := r.scene.Without(aurora.FieldName)

	report := r.push(t, foreign, nameless, r.scene)
	if o := report.Results[0]; o.Outcome != push.Refused || !strings.Contains(o.Reason, "does not have the plugin") {
		t.Errorf("a plugin the controller lacks: %+v", o)
	}
	if o := report.Results[1]; o.Outcome != push.Refused || !strings.Contains(o.Reason, "no name") {
		t.Errorf("no name: %+v", o)
	}
	// the one that can go still goes
	if o := report.Results[2]; o.Outcome != push.Added {
		t.Errorf("the good one: %+v", o)
	}
	if report.Wrote() != 1 || report.Err() == nil {
		t.Errorf("wrote %d, err %v", report.Wrote(), report.Err())
	}
}

// A file saved from a write carries the command it was sent with, which is
// no part of the scene.
func TestACommandInTheSceneIsNotPartOfIt(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.ctl.PutEffect(r.scene)
	asSent, err := r.scene.With("command", "add")
	if err != nil {
		t.Fatal(err)
	}
	if report := r.push(t, asSent); report.Results[0].Outcome != push.Unchanged {
		t.Errorf("report: %+v", report)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	t.Parallel()

	var would []aurora.Request
	r := newRig(t, aurora.WithDryRun(func(q aurora.Request) { would = append(would, q) }))
	r.opt.DryRun = true

	report := r.push(t, r.scene)
	if !report.DryRun || report.Results[0].Outcome != push.Added {
		t.Fatalf("report: %+v", report)
	}
	if len(would) != 1 || !strings.Contains(string(would[0].Body), `"command":"add"`) {
		t.Errorf("what it would send: %+v", would)
	}
	if _, ok := r.ctl.Effect(scene); ok || len(r.ctl.Writes()) != 0 {
		t.Error("a dry run wrote to the controller")
	}
	// it says where the backup would go, and takes none
	if report.Backup == "" {
		t.Error("a dry run should say where the backup would go")
	}
	if _, err := os.Stat(report.Backup); !os.IsNotExist(err) {
		t.Errorf("a dry run took a backup: %v", err)
	}
}

func TestNothingIsWrittenWithoutABackup(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	// somewhere a backup cannot go: a file where the directory should be
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.opt.BackupRoot = blocked

	report, err := push.Scenes(t.Context(), r.c, []aurora.Effect{r.scene}, r.opt)
	if err == nil || !strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("want an error saying nothing was written, got %v", err)
	}
	if report.Backup != "" || len(r.ctl.Writes()) != 0 {
		t.Errorf("backup %q, writes %+v", report.Backup, r.ctl.Writes())
	}
}

func TestAControllerThatCannotBeRead(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"requestAll", "requestPlugins"} {
		r := newRig(t)
		r.ctl.FailCommand(command, http.StatusInternalServerError)
		if _, err := push.Scenes(t.Context(), r.c, []aurora.Effect{r.scene}, r.opt); err == nil || !strings.Contains(err.Error(), "bedroom") {
			t.Errorf("%s failing: %v", command, err)
		}
		if len(r.ctl.Writes()) != 0 {
			t.Errorf("%s failing: wrote %+v", command, r.ctl.Writes())
		}
	}
}

func TestAControllerThatRefusesTheScene(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	other, _ := auroratest.New(t).Effect("Flames")
	r.ctl.RemoveEffect("Flames")
	r.ctl.FailCommand("add", http.StatusUnprocessableEntity)

	report := r.push(t, r.scene, other)
	for _, res := range report.Results {
		if res.Outcome != push.Failed || !strings.Contains(res.Reason, "HTTP 422") {
			t.Errorf("%s: %+v", res.Scene, res)
		}
	}
	if report.Wrote() != 0 || report.Err() == nil {
		t.Errorf("wrote %d, err %v", report.Wrote(), report.Err())
	}
	// it was backed up all the same: the attempt was a write
	if _, err := backup.Read(report.Backup); err != nil {
		t.Errorf("the backup: %v", err)
	}
}

// A controller may store a scene its own way. That is not a failure, and it
// is said.
func TestSaysWhenASceneReadsBackDifferently(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.ctl.StoreAs(func(e aurora.Effect) aurora.Effect { return e.Without("hasOverlay") })

	report := r.push(t, r.scene)
	res := report.Results[0]
	if res.Outcome != push.Added || len(res.ReadsBack) != 1 || res.ReadsBack[0] != "hasOverlay" {
		t.Errorf("report: %+v", res)
	}
	if report.Err() != nil || report.Wrote() != 1 {
		t.Errorf("it is on the controller, so it is not an error: %v", report.Err())
	}
}

// A controller that says it took a scene and then does not have it has not
// taken it.
func TestASceneThatDoesNotComeBack(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.ctl.StoreAs(func(e aurora.Effect) aurora.Effect { return e.WithName("something else") })

	report := r.push(t, r.scene)
	if res := report.Results[0]; res.Outcome != push.Failed || !strings.Contains(res.Reason, "does not give it back") {
		t.Errorf("report: %+v", res)
	}
	if report.Err() == nil {
		t.Error("want an error")
	}
}
