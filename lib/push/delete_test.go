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

func (r rig) delete(t *testing.T, names ...string) push.Report {
	t.Helper()

	report, err := push.Delete(t.Context(), r.c, names, r.opt)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	return report
}

func TestDeleteNeedsToBeTold(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	report := r.delete(t, "Flames", "Forest")
	for _, res := range report.Results {
		if res.Outcome != push.Refused || !strings.Contains(res.Reason, "--force") {
			t.Errorf("%s: %+v", res.Scene, res)
		}
	}
	if report.Err() == nil || report.Wrote() != 0 || report.Backup != "" {
		t.Errorf("report: %+v", report)
	}
	if len(r.ctl.EffectNames()) != 16 || len(r.ctl.Writes()) != 0 {
		t.Error("something was deleted without being told to")
	}
	if entries, _ := os.ReadDir(r.opt.BackupRoot); len(entries) != 0 {
		t.Error("a backup was taken with nothing to delete")
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	r := newRig(t)
	r.opt.Force = true
	// the rig's controller lost one scene; it is running another
	running := r.ctl.Selected()
	if err := r.c.SelectEffect(t.Context(), "Nemo"); err != nil {
		t.Fatal(err)
	}
	_ = running

	report := r.delete(t, "Flames", "Nemo", "No Such Scene", "Forest")
	want := map[string]push.Outcome{"Flames": push.Deleted, "Forest": push.Deleted, "Nemo": push.Refused, "No Such Scene": push.Refused}
	for _, res := range report.Results {
		if res.Outcome != want[res.Scene] {
			t.Errorf("%s: %+v", res.Scene, res)
		}
	}
	// the one that is running is never deleted from under the controller
	if res := report.Results[1]; !strings.Contains(res.Reason, "running") {
		t.Errorf("the running scene: %+v", res)
	}
	if res := report.Results[2]; !strings.Contains(res.Reason, "no scene of that name") {
		t.Errorf("a scene that is not there: %+v", res)
	}
	if report.Wrote() != 2 || report.Err() == nil {
		t.Errorf("wrote %d, err %v", report.Wrote(), report.Err())
	}

	left := r.ctl.EffectNames()
	if len(left) != 14 || slices.Contains(left, "Flames") || slices.Contains(left, "Forest") || !slices.Contains(left, "Nemo") {
		t.Errorf("left on the controller: %v", left)
	}
	// everything that went is in the backup taken first, and can be put back from it
	b, err := backup.Read(report.Backup)
	if err != nil || len(b.Scenes) != 16 {
		t.Fatalf("the backup: %v", err)
	}
	var gone []aurora.Effect
	for _, e := range b.Scenes {
		if e.Name() == "Flames" || e.Name() == "Forest" {
			gone = append(gone, e)
		}
	}
	back, err := push.Scenes(t.Context(), r.c, gone, r.opt)
	if err != nil || back.Wrote() != 2 || len(r.ctl.EffectNames()) != 16 {
		t.Errorf("putting them back: %+v, %v", back, err)
	}
}

func TestDeleteDryRun(t *testing.T) {
	t.Parallel()

	var would []aurora.Request
	r := newRig(t, aurora.WithDryRun(func(q aurora.Request) { would = append(would, q) }))
	r.opt.Force, r.opt.DryRun = true, true

	report := r.delete(t, "Flames")
	if !report.DryRun || report.Results[0].Outcome != push.Deleted || report.Backup == "" {
		t.Fatalf("report: %+v", report)
	}
	if len(would) != 1 || string(would[0].Body) != `{"write":{"command":"delete","animName":"Flames"}}` {
		t.Errorf("what it would send: %+v", would)
	}
	if len(r.ctl.EffectNames()) != 16 || len(r.ctl.Writes()) != 0 {
		t.Error("a dry run deleted a scene")
	}
	if _, err := os.Stat(report.Backup); !os.IsNotExist(err) {
		t.Error("a dry run took a backup")
	}
}

func TestDeleteWhenTheControllerWillNot(t *testing.T) {
	t.Parallel()

	// nothing is deleted without a backup
	r := newRig(t)
	r.opt.Force = true
	blocked := t.TempDir() + "/file"
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.opt.BackupRoot = blocked
	if _, err := push.Delete(t.Context(), r.c, []string{"Flames"}, r.opt); err == nil || !strings.Contains(err.Error(), "nothing was deleted") {
		t.Errorf("with nowhere to back up to: %v", err)
	}
	if len(r.ctl.EffectNames()) != 16 {
		t.Error("a scene was deleted without a backup")
	}

	// a controller that refuses the delete
	r = newRig(t)
	r.opt.Force = true
	r.ctl.FailCommand("delete", http.StatusInternalServerError)
	if res := r.delete(t, "Flames").Results[0]; res.Outcome != push.Failed || !strings.Contains(res.Reason, "HTTP 500") {
		t.Errorf("a refused delete: %+v", res)
	}

	// a controller that cannot be read is not deleted from
	r = newRig(t)
	r.opt.Force = true
	r.ctl.Fail(http.MethodGet, "/effects/effectsList", http.StatusInternalServerError)
	if _, err := push.Delete(t.Context(), r.c, []string{"Flames"}, r.opt); err == nil {
		t.Error("want an error from a controller that cannot be read")
	}
	r.ctl.Fail(http.MethodGet, "/effects/effectsList", 0)
	r.ctl.Fail(http.MethodGet, "/effects/select", http.StatusInternalServerError)
	if _, err := push.Delete(t.Context(), r.c, []string{"Flames"}, r.opt); err == nil || len(r.ctl.Writes()) != 0 {
		t.Errorf("with what is running unknown: %v", err)
	}
}
