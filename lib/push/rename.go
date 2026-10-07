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

// Rename gives a scene on the controller behind c another name, and reports
// what happened. Nothing is lost by a rename, so it needs no Force, but it is
// as careful as any write: a name another scene already has is refused, since
// the controller would be left with one of them; the scene that is running is
// left alone, since what the controller then says is running is not known;
// the whole controller is backed up first; and afterwards the scene is read
// back under its new name and compared with what it was.
//
// It returns an error only when it could not get as far as deciding.
func Rename(ctx context.Context, c *aurora.Client, name, newName string, opt Options) (Report, error) {
	report := Report{Controller: opt.Controller, DryRun: opt.DryRun}
	res := Result{Scene: name, To: newName}

	held, err := c.EffectNames(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s holds: %w", opt.Controller, err)
	}
	running, err := c.SelectedEffect(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s is running: %w", opt.Controller, err)
	}

	newName = strings.TrimSpace(newName)
	switch {
	case !slices.Contains(held, name):
		res.Outcome, res.Reason = Refused, "the controller holds no scene of that name"
	case newName == "":
		res.Outcome, res.Reason = Refused, "the new name is empty"
	case newName == name:
		res.Outcome, res.Reason = Unchanged, "that is its name already"
	case slices.Contains(held, newName):
		res.Outcome, res.Reason = Refused, fmt.Sprintf("the controller already holds a scene called %q", newName)
	case name == running:
		res.Outcome, res.Reason = Refused, "it is the scene that is running: start another first"
	default:
		res.Outcome = Renamed
	}
	if res.Outcome != Renamed {
		report.Results = append(report.Results, res)
		return report, nil
	}

	// what it was, to compare with what it reads back as
	before, err := c.Effect(ctx, name)
	if err != nil {
		return report, fmt.Errorf("reading %q from %s: %w", name, opt.Controller, err)
	}

	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	report.Backup = backup.Dir(opt.BackupRoot, opt.Controller, now())
	if !opt.DryRun {
		if _, err := backup.Take(ctx, c, report.Backup); err != nil {
			report.Backup = ""
			return report, fmt.Errorf("backing up %s before renaming on it: %w (nothing was renamed)", opt.Controller, err)
		}
	}

	if err := c.RenameEffect(ctx, name, newName); err != nil {
		res.Outcome, res.Reason = Failed, err.Error()
	}
	report.Results = append(report.Results, res)
	if opt.DryRun || res.Outcome != Renamed {
		return report, nil
	}

	// the controller says yes to a rename; whether the scene is now under the
	// new name, and still the same scene, is for the controller to say
	left, err := c.EffectNames(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s holds after renaming: %w", opt.Controller, err)
	}
	back := &report.Results[0]
	switch {
	case !slices.Contains(left, newName):
		back.Outcome, back.Reason = Failed, "the controller took the rename and holds no scene of the new name"
	case slices.Contains(left, name):
		back.Outcome, back.Reason = Failed, "the controller took the rename and still holds the scene under its old name"
	default:
		if after, err := c.Effect(ctx, newName); err != nil {
			back.Outcome, back.Reason = Failed, "the controller took it but does not give it back: "+err.Error()
		} else {
			back.ReadsBack = after.SceneDifferences(before.WithName(newName))
		}
	}

	return report, nil
}
