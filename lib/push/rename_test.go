package push_test

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/lib/push"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

func (r rig) rename(t *testing.T, name, newName string) push.Result {
	t.Helper()

	report, err := push.Rename(t.Context(), r.c, name, newName, r.opt)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("report: %+v", report)
	}

	return report.Results[0]
}

func TestRenameRefusals(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	// the rig's controller lost the scene it was running; run one it holds
	if err := r.c.SelectEffect(t.Context(), "Nemo"); err != nil {
		t.Fatal(err)
	}
	writes := len(r.ctl.Writes()) // the select is one
	for _, tc := range []struct{ name, to, reason string }{
		{"No Such Scene", "Anything", "no scene of that name"},
		{"Flames", "", "empty"},
		{"Flames", "Forest", `already holds a scene called "Forest"`},
		{"Nemo", "Something Else", "running"},
	} {
		res := r.rename(t, tc.name, tc.to)
		if res.Outcome != push.Refused || !strings.Contains(res.Reason, tc.reason) {
			t.Errorf("%q -> %q: %+v", tc.name, tc.to, res)
		}
	}

	// its own name is not a change
	if res := r.rename(t, "Flames", "Flames"); res.Outcome != push.Unchanged {
		t.Errorf("to its own name: %+v", res)
	}

	if len(r.ctl.EffectNames()) != 16 || len(r.ctl.Writes()) != writes {
		t.Error("something was renamed that should not have been")
	}
	if entries, _ := os.ReadDir(r.opt.BackupRoot); len(entries) != 0 {
		t.Error("a backup was taken with nothing to rename")
	}
}

func TestRename(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	before, ok := r.ctl.Effect("Flames")
	if !ok {
		t.Fatal("the fixture has lost Flames")
	}

	report, err := push.Rename(t.Context(), r.c, "Flames", "Fire", r.opt)
	if err != nil {
		t.Fatal(err)
	}
	res := report.Results[0]
	if res.Outcome != push.Renamed || res.Scene != "Flames" || res.To != "Fire" || len(res.ReadsBack) != 0 {
		t.Errorf("result: %+v", res)
	}
	if report.Wrote() != 1 || report.Err() != nil {
		t.Errorf("wrote %d, err %v", report.Wrote(), report.Err())
	}

	held := r.ctl.EffectNames()
	if len(held) != 16 || slices.Contains(held, "Flames") || !slices.Contains(held, "Fire") {
		t.Errorf("held: %v", held)
	}
	// the same scene, under the new name
	after, _ := r.ctl.Effect("Fire")
	if !after.SameScene(before.WithName("Fire")) {
		t.Errorf("the scene changed as it was renamed: %v", after.SceneDifferences(before.WithName("Fire")))
	}
	// and the backup taken first holds it under its old name
	b, err := backup.Read(report.Backup)
	if err != nil || len(b.Scenes) != 16 {
		t.Fatalf("the backup: %v", err)
	}
	if !slices.ContainsFunc(b.Scenes, func(e aurora.Effect) bool { return e.Name() == "Flames" }) {
		t.Error("the backup does not hold the scene under its old name")
	}

	// and back again
	if res := r.rename(t, "Fire", "Flames"); res.Outcome != push.Renamed {
		t.Errorf("renaming back: %+v", res)
	}
}

func TestRenameDryRun(t *testing.T) {
	t.Parallel()

	var would []aurora.Request
	r := newRig(t, aurora.WithDryRun(func(q aurora.Request) { would = append(would, q) }))
	r.opt.DryRun = true

	report, err := push.Rename(t.Context(), r.c, "Flames", "Fire", r.opt)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun || report.Results[0].Outcome != push.Renamed || report.Backup == "" {
		t.Fatalf("report: %+v", report)
	}
	if len(would) != 1 || string(would[0].Body) != `{"write":{"command":"rename","animName":"Flames","newName":"Fire"}}` {
		t.Errorf("what it would send: %+v", would)
	}
	if held := r.ctl.EffectNames(); !slices.Contains(held, "Flames") || len(r.ctl.Writes()) != 0 {
		t.Error("a dry run renamed a scene")
	}
	if _, err := os.Stat(report.Backup); !os.IsNotExist(err) {
		t.Error("a dry run took a backup")
	}
}

func TestRenameWhenTheControllerWillNot(t *testing.T) {
	t.Parallel()

	// nothing is renamed without a backup
	r := newRig(t)
	blocked := t.TempDir() + "/file"
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.opt.BackupRoot = blocked
	if _, err := push.Rename(t.Context(), r.c, "Flames", "Fire", r.opt); err == nil || !strings.Contains(err.Error(), "nothing was renamed") {
		t.Errorf("with nowhere to back up to: %v", err)
	}
	if held := r.ctl.EffectNames(); !slices.Contains(held, "Flames") {
		t.Error("a scene was renamed without a backup")
	}

	// a controller that refuses the rename
	r = newRig(t)
	r.ctl.FailCommand("rename", http.StatusInternalServerError)
	if res := r.rename(t, "Flames", "Fire"); res.Outcome != push.Failed || !strings.Contains(res.Reason, "HTTP 500") {
		t.Errorf("a refused rename: %+v", res)
	}

	// a controller that cannot be read is not renamed on
	r = newRig(t)
	r.ctl.Fail(http.MethodGet, "/effects/effectsList", http.StatusInternalServerError)
	if _, err := push.Rename(t.Context(), r.c, "Flames", "Fire", r.opt); err == nil {
		t.Error("want an error from a controller that cannot be read")
	}
	r.ctl.Fail(http.MethodGet, "/effects/effectsList", 0)
	r.ctl.Fail(http.MethodGet, "/effects/select", http.StatusInternalServerError)
	if _, err := push.Rename(t.Context(), r.c, "Flames", "Fire", r.opt); err == nil || len(r.ctl.Writes()) != 0 {
		t.Errorf("with what is running unknown: %v", err)
	}
}
