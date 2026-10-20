package plakar

import (
	"context"
	"fmt"
	"strconv"

	"github.com/opensvc/om3/v3/core/backup"
)

// Prune removes the snapshots matching the filter that the keep retention
// does not keep.
//
// The retention "7d 4w 6m" runs "prune -tag <filter> -days 7 -per-day 1
// -weeks 4 -per-week 1 -months 6 -per-month 1 -apply": plakar keeps the
// most recent snapshot of each period, and removes the others. Only the
// snapshots matching the filter are evaluated. The space is reclaimed by
// "plakar maintenance", after its grace delay.
func (t *T) Prune(ctx context.Context, filter backup.Tags, keep backup.Retention) error {
	if keep.IsZero() {
		return nil
	}
	if len(filter) == 0 {
		return fmt.Errorf("store %s: refuse to prune without a filter", t.name)
	}
	if err := filter.Validate(); err != nil {
		return err
	}
	args := []string{"prune"}
	args = append(args, tagArgs(filter)...)
	args = append(args, retentionArgs(keep)...)
	args = append(args, "-apply")
	if err := t.run(ctx, call{args: args}); err != nil {
		return fmt.Errorf("store %s: prune %s: %w", t.name, filter, err)
	}
	return nil
}

func retentionArgs(keep backup.Retention) []string {
	l := make([]string, 0, 8)
	for _, e := range []struct {
		n      int
		period string
		per    string
	}{
		{keep.Days, "-days", "-per-day"},
		{keep.Weeks, "-weeks", "-per-week"},
		{keep.Months, "-months", "-per-month"},
		{keep.Years, "-years", "-per-year"},
	} {
		if e.n > 0 {
			l = append(l, e.period, strconv.Itoa(e.n), e.per, "1")
		}
	}
	return l
}
