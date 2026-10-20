package plakar

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/opensvc/om3/v3/core/store"
)

// List returns the snapshots matching all the filter tags, the most recent
// first, or all the snapshots of the store with an empty filter.
func (t *T) List(ctx context.Context, filter store.Tags) ([]store.Snapshot, error) {
	return t.list(ctx, filter, false)
}

// latest returns the most recent snapshot matching all the filter tags.
func (t *T) latest(ctx context.Context, filter store.Tags) (store.Snapshot, error) {
	l, err := t.list(ctx, filter, true)
	if err != nil {
		return store.Snapshot{}, err
	}
	s, ok := store.Latest(l)
	if !ok {
		return store.Snapshot{}, fmt.Errorf("store %s: %w matching %s", t.name, store.ErrNoSnapshot, filter)
	}
	return s, nil
}

func (t *T) list(ctx context.Context, filter store.Tags, latest bool) ([]store.Snapshot, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	args := []string{"ls", "-uuid", "-tags"}
	if latest {
		args = append(args, "-latest")
	}
	args = append(args, tagArgs(filter)...)
	l := make([]store.Snapshot, 0)
	onLine := func(line string) {
		s, err := parseListLine(line)
		if err != nil {
			t.log.Debugf("ls: skip line %q: %s", line, err)
			return
		}
		l = append(l, s)
	}
	if err := t.run(ctx, call{args: args, onLine: onLine}); err != nil {
		return nil, fmt.Errorf("store %s: list: %w", t.name, err)
	}
	slices.SortStableFunc(l, func(a, b store.Snapshot) int {
		return b.Time.Compare(a.Time)
	})
	return l, nil
}

// tagArgs returns the -tag filter of a ls, check or prune command, with
// which plakar keeps the snapshots having all the tags.
func tagArgs(filter store.Tags) []string {
	if len(filter) == 0 {
		return nil
	}
	return []string{"-tag", filter.String()}
}

// parseListLine parses a line of "plakar ls -uuid -tags":
//
//	<RFC3339 time> <id> <size> <unit> <duration> <root>[ tags=<k>=<v>,...]
//
// the size being humanized, as in "1.2 MiB", and the tags absent from the
// snapshots having none.
func parseListLine(line string) (store.Snapshot, error) {
	var s store.Snapshot
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return s, fmt.Errorf("not enough fields")
	}
	tm, err := time.Parse(time.RFC3339, fields[0])
	if err != nil {
		return s, err
	}
	if !isHex(fields[1]) {
		return s, fmt.Errorf("invalid snapshot id %q", fields[1])
	}
	s.Time = tm
	s.ID = store.SnapshotID(fields[1])
	rest := fields[5:]
	s.Tags = make(store.Tags)
	if last := rest[len(rest)-1]; strings.HasPrefix(last, "tags=") {
		rest = rest[:len(rest)-1]
		for _, tag := range strings.Split(strings.TrimPrefix(last, "tags="), ",") {
			if k, v, ok := strings.Cut(tag, "="); ok {
				s.Tags[k] = v
			}
		}
	}
	s.Root = strings.Join(rest, " ")
	return s, nil
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
