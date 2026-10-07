// Package push puts scenes onto a controller without losing anything that is
// already there. Every way taproot writes a scene goes through it: a pushed
// file, a copy between controllers, a restore, the page's copy button.
//
// A controller replaces a scene of the same name without a word, and has no
// undo. So before anything is sent this package looks at what the controller
// holds: a scene that is already there and the same is left alone, one that
// is there and different is refused unless told to replace it, and before the
// first write of a run everything the controller holds is backed up. After a
// write the scene is read back and compared with what was sent.
package push

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Outcome is what happened to one scene.
type Outcome string

// The outcomes.
const (
	Added     Outcome = "added"     // it was not there, and now is
	Replaced  Outcome = "replaced"  // a different scene of that name was there, and Force replaced it
	Deleted   Outcome = "deleted"   // it was there, and Force took it off
	Renamed   Outcome = "renamed"   // it was there, and is now under the name in To
	Unchanged Outcome = "unchanged" // the same scene was already there
	Refused   Outcome = "refused"   // nothing was sent: Reason says why
	Failed    Outcome = "failed"    // the controller did not take it: Reason says why
)

// Result is what happened to one scene.
type Result struct {
	Scene   string  `json:"scene"`
	Outcome Outcome `json:"outcome"`
	// To is the name a scene was renamed to, for a rename only.
	To string `json:"to,omitempty"`
	// Reason says why a scene was refused or failed, in words for a person.
	Reason string `json:"reason,omitempty"`
	// Differs names the fields in which the scene on the controller differed
	// from the one pushed, for a scene that was refused or replaced.
	Differs []string `json:"differs,omitempty"`
	// ReadsBack names the fields in which the scene read back after the
	// write differs from what was sent, beyond the ones a controller is known
	// to add by itself. It is empty when the scene arrived as it was sent; a
	// field named here is something the controller changed, to be looked at.
	ReadsBack []string `json:"readsBack,omitempty"`
}

// Report is what a push did.
type Report struct {
	Controller string   `json:"controller"`
	Results    []Result `json:"results"`
	// Backup is the directory the controller was backed up to before the
	// first write, empty when nothing needed writing. In a dry run it is
	// where the backup would have gone.
	Backup string `json:"backup,omitempty"`
	DryRun bool   `json:"dryRun,omitempty"`
}

// Wrote is how many scenes were added, replaced, deleted or renamed.
func (r Report) Wrote() int {
	n := 0
	for _, res := range r.Results {
		if res.Outcome == Added || res.Outcome == Replaced || res.Outcome == Deleted || res.Outcome == Renamed {
			n++
		}
	}

	return n
}

// Err is an error naming the scenes that were refused or failed, nil when
// every scene is on the controller.
func (r Report) Err() error {
	var bad []string
	for _, res := range r.Results {
		if res.Outcome == Refused || res.Outcome == Failed {
			bad = append(bad, fmt.Sprintf("%q %s: %s", res.Scene, res.Outcome, res.Reason))
		}
	}
	if len(bad) == 0 {
		return nil
	}

	return fmt.Errorf("%s: %s", r.Controller, strings.Join(bad, "; "))
}

// Options say how to push.
type Options struct {
	// Controller is the controller's name, for the report and for the
	// directory its backup goes in.
	Controller string
	// BackupRoot is the directory backups are kept under.
	BackupRoot string
	// Force replaces a scene that is already there and different.
	Force bool
	// DryRun says the client was made with aurora.WithDryRun: nothing is
	// backed up or read back, since nothing is written.
	DryRun bool
	// Now is the time the backup is filed under; time.Now when nil.
	Now func() time.Time
}

// Scenes puts scenes onto the controller behind c, in order, and reports what
// happened to each. It returns an error only when it could not get as far as
// deciding: a controller that cannot be read, a backup that cannot be taken.
// A scene the controller refuses is in the report, and the rest still go.
func Scenes(ctx context.Context, c *aurora.Client, scenes []aurora.Effect, opt Options) (Report, error) {
	report := Report{Controller: opt.Controller, DryRun: opt.DryRun}

	held, err := c.Effects(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s holds: %w", opt.Controller, err)
	}
	plugins, err := c.Plugins(ctx)
	if err != nil {
		return report, fmt.Errorf("reading the plugins %s has: %w", opt.Controller, err)
	}

	// decide everything first, so the backup is taken once and only when something will be written
	var write []int
	scenes = slices.Clone(scenes)
	for n, e := range scenes {
		// a file saved from a write carries the command it went with, which is no part of the scene
		e = e.Without("command")
		scenes[n] = e
		res := Result{Scene: e.Name()}
		i := slices.IndexFunc(held, func(h aurora.Effect) bool { return h.Name() == e.Name() })
		switch {
		case e.Name() == "":
			res.Outcome, res.Reason = Refused, "the scene has no name"
		case e.PluginUUID() != "" && !slices.ContainsFunc(plugins, func(p aurora.Plugin) bool { return p.UUID == e.PluginUUID() }):
			res.Outcome = Refused
			res.Reason = fmt.Sprintf("the controller does not have the plugin the scene runs (%s), so it could not play it", e.PluginUUID())
		// the same scene, whatever the firmware holding it has added of its own
		case i >= 0 && held[i].SameScene(e):
			res.Outcome = Unchanged
		case i >= 0 && !opt.Force:
			res.Outcome, res.Differs = Refused, held[i].SceneDifferences(e)
			res.Reason = "a different scene of that name is already there (differs in " + strings.Join(res.Differs, ", ") + "): --force replaces it"
		case i >= 0:
			res.Outcome, res.Differs = Replaced, held[i].SceneDifferences(e)
			write = append(write, len(report.Results))
		default:
			res.Outcome = Added
			write = append(write, len(report.Results))
		}
		report.Results = append(report.Results, res)
	}
	if len(write) == 0 {
		return report, nil
	}

	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	report.Backup = backup.Dir(opt.BackupRoot, opt.Controller, now())
	if !opt.DryRun {
		if _, err := backup.Take(ctx, c, report.Backup); err != nil {
			report.Backup = ""
			return report, fmt.Errorf("backing up %s before writing to it: %w (nothing was written)", opt.Controller, err)
		}
	}

	for _, i := range write {
		res := &report.Results[i]
		e := scenes[i]
		if err := c.AddEffect(ctx, e); err != nil {
			res.Outcome, res.Reason = Failed, err.Error()
			continue
		}
		if opt.DryRun {
			continue
		}
		back, err := c.Effect(ctx, e.Name())
		if err != nil {
			res.Outcome, res.Reason = Failed, "the controller took it but does not give it back: "+err.Error()
			continue
		}
		res.ReadsBack = back.SceneDifferences(e)
	}

	return report, nil
}
