package object

import (
	"context"
	"fmt"
	"strings"

	"github.com/opensvc/om3/v3/core/actioncontext"
	"github.com/opensvc/om3/v3/core/driver"
	"github.com/opensvc/om3/v3/core/resource"
	"github.com/opensvc/om3/v3/core/resourceselector"
	"github.com/opensvc/om3/v3/core/status"
)

type (
	// SyncRestoreOptions are the options of the "sync restore" action.
	SyncRestoreOptions struct {
		resource.RestoreOptions

		// Stop stops the instance before an in place restore.
		Stop bool
	}
)

var (
	// restoreGuardGroups are the driver groups whose resources being up
	// refuse an in place restore: the application would read data being
	// overwritten.
	restoreGuardGroups = []driver.Group{
		driver.GroupApp,
		driver.GroupContainer,
		driver.GroupShare,
	}
)

// SyncRestore restores the data of the object from the snapshots the sync
// resources implementing Restore made.
//
// The resource selection does not select the sync resources the action runs
// on, but the data resources to restore: "--rid fs#1" restores fs#1. The sync
// resources of the selection, if any, are the ones the action runs on.
//
// An in place restore is refused if an app, container or share resource is
// up, unless --stop stops the instance first or --force.
func (t *actor) SyncRestore(ctx context.Context, opts SyncRestoreOptions) error {
	ctx = actioncontext.WithProps(ctx, actioncontext.SyncRestore)
	if err := t.validateAction(); err != nil {
		return err
	}
	t.setenv("sync_restore", false)
	unlock, err := t.lockAction(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return t.lockedSyncRestore(ctx, opts)
}

func (t *actor) lockedSyncRestore(ctx context.Context, opts SyncRestoreOptions) error {
	ctx, err := t.splitRestoreSelection(ctx, &opts.RestoreOptions)
	if err != nil {
		return err
	}
	if opts.Into == "" {
		if err := t.guardInPlaceRestore(ctx, opts.Stop); err != nil {
			return err
		}
	}
	return t.action(ctx, func(ctx context.Context, r resource.Driver) error {
		return resource.Restore(ctx, r, opts.RestoreOptions)
	})
}

// splitRestoreSelection moves the data resources of the --rid selection to
// the restore options, and returns a context selecting only its sync
// resources, or all the resources if it has none.
func (t *actor) splitRestoreSelection(ctx context.Context, opts *resource.RestoreOptions) (context.Context, error) {
	rid := actioncontext.RID(ctx)
	if rid == "" {
		return ctx, nil
	}
	selected := resourceselector.New(t, resourceselector.WithRID(rid)).Resources()
	if len(selected) == 0 {
		return ctx, fmt.Errorf("resource does not exist")
	}
	syncRIDs := make([]string, 0)
	for _, r := range selected {
		if r.ID().DriverGroup() == driver.GroupSync {
			syncRIDs = append(syncRIDs, r.RID())
		} else {
			opts.RIDs = append(opts.RIDs, r.RID())
		}
	}
	return actioncontext.WithRID(ctx, strings.Join(syncRIDs, ",")), nil
}

// guardInPlaceRestore refuses an in place restore while an app, container
// or share resource is up, unless stop is set, in which case it stops the
// instance, or the action is forced.
func (t *actor) guardInPlaceRestore(ctx context.Context, stop bool) error {
	up := make([]string, 0)
	for _, r := range t.ResourcesByDrivergroups(restoreGuardGroups) {
		if r.IsDisabled() {
			continue
		}
		switch resource.EvalStatus(ctx, r) {
		case status.Up, status.StandbyUp, status.Warn:
			up = append(up, r.RID())
		}
	}
	if len(up) == 0 {
		return nil
	}
	switch {
	case stop:
		t.log.Infof("stop the instance before the in place restore (%s up)", strings.Join(up, ","))
		stopCtx := actioncontext.WithProps(ctx, actioncontext.Stop)
		stopCtx = actioncontext.WithRID(stopCtx, "")
		return t.lockedStop(stopCtx)
	case actioncontext.IsForce(ctx):
		t.log.Warnf("forced in place restore while %s up", strings.Join(up, ","))
		return nil
	default:
		return fmt.Errorf("refuse to restore in place while %s up: use --stop, --force or --into", strings.Join(up, ","))
	}
}
