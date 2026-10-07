package push

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/katbyte/nanoleaf-aurora-taproot/lib/backup"
	"github.com/katbyte/nanoleaf-aurora-taproot/sdk/aurora"
)

// Delete takes scenes off the controller behind c, by name, and reports what
// happened to each. A controller has no undo and no bin, so this is as
// careful as adding: nothing is deleted unless Force says so, the scene that
// is running is never deleted from under it, the whole controller is backed
// up before the first delete, and afterwards the controller is asked whether
// the scene has really gone. What a backup holds, taproot restore puts back.
//
// It returns an error only when it could not get as far as deciding. A scene
// the controller will not let go of is in the report, and the rest still go.
func Delete(ctx context.Context, c *aurora.Client, names []string, opt Options) (Report, error) {
	report := Report{Controller: opt.Controller, DryRun: opt.DryRun}

	held, err := c.EffectNames(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s holds: %w", opt.Controller, err)
	}
	running, err := c.SelectedEffect(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s is running: %w", opt.Controller, err)
	}

	var doomed []int
	for _, name := range names {
		res := Result{Scene: name}
		switch {
		case !slices.Contains(held, name):
			res.Outcome, res.Reason = Refused, "the controller holds no scene of that name"
		case name == running:
			res.Outcome, res.Reason = Refused, "it is the scene that is running: start another first"
		case !opt.Force:
			res.Outcome, res.Reason = Refused, "not deleted without --force"
		default:
			res.Outcome = Deleted
			doomed = append(doomed, len(report.Results))
		}
		report.Results = append(report.Results, res)
	}
	if len(doomed) == 0 {
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
			return report, fmt.Errorf("backing up %s before deleting from it: %w (nothing was deleted)", opt.Controller, err)
		}
	}

	for _, i := range doomed {
		res := &report.Results[i]
		if err := c.DeleteEffect(ctx, res.Scene); err != nil {
			res.Outcome, res.Reason = Failed, err.Error()
		}
	}
	if opt.DryRun {
		return report, nil
	}

	// the controller says yes to a delete; whether the scene has gone is for the list to say
	left, err := c.EffectNames(ctx)
	if err != nil {
		return report, fmt.Errorf("reading what %s holds after deleting: %w", opt.Controller, err)
	}
	for _, i := range doomed {
		if res := &report.Results[i]; res.Outcome == Deleted && slices.Contains(left, res.Scene) {
			res.Outcome, res.Reason = Failed, "the controller took the delete and still holds the scene"
		}
	}

	return report, nil
}
