package plakar

import (
	"context"
	"fmt"

	"github.com/opensvc/om3/v3/core/store"
)

// Check verifies the integrity of all the snapshots of the store.
func (t *T) Check(ctx context.Context) error {
	if err := t.run(ctx, call{args: []string{"check"}, quiet: true}); err != nil {
		return fmt.Errorf("store %s: check: %w", t.name, err)
	}
	return nil
}

// CheckFiltered verifies the integrity of the latest snapshot matching the
// filter.
//
// plakar checks nothing and succeeds when no snapshot matches, so the
// snapshot is looked up first.
func (t *T) CheckFiltered(ctx context.Context, filter store.Tags) error {
	if _, err := t.latest(ctx, filter); err != nil {
		return err
	}
	args := []string{"check", "-latest"}
	args = append(args, tagArgs(filter)...)
	if err := t.run(ctx, call{args: args, quiet: true}); err != nil {
		return fmt.Errorf("store %s: check %s: %w", t.name, filter, err)
	}
	return nil
}
