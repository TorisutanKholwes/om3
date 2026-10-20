package plakar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/store"
)

func TestPrune(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	filter := store.Tags{"path": "test/svc/app1", "rid": "fs#1"}
	require.NoError(t, s.Prune(context.Background(), filter, store.Retention{Days: 7, Weeks: 4, Months: 6}))
	require.Equal(t, []string{
		"prune -tag path=test/svc/app1,rid=fs#1 -days 7 -per-day 1 -weeks 4 -per-week 1 -months 6 -per-month 1 -apply",
	}, f.calls())

	require.NoError(t, s.Prune(context.Background(), filter, store.Retention{Years: 2}))
	require.Equal(t, "prune -tag path=test/svc/app1,rid=fs#1 -years 2 -per-year 1 -apply", f.calls()[1])
}

func TestPruneWithoutKeepOrFilter(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.NoError(t, s.Prune(context.Background(), store.Tags{"path": "p"}, store.Retention{}))
	require.Error(t, s.Prune(context.Background(), nil, store.Retention{Days: 1}))
	require.Empty(t, f.calls())
}

func TestCheck(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.NoError(t, s.Check(context.Background()))
	require.Equal(t, []string{"check"}, f.calls())

	f.answer("check", "check failed for 1 snapshot(s)\n", 65)
	require.ErrorIs(t, s.Check(context.Background()), ErrIntegrity)
}

func TestCheckFiltered(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.ErrorIs(t, s.CheckFiltered(context.Background(), store.Tags{"pass": "p1"}), store.ErrNoSnapshot)

	f.answer("ls", "2026-10-14T09:00:00Z "+id1+"   1 KiB        0s / tags=pass=p1\n", 0)
	require.NoError(t, s.CheckFiltered(context.Background(), store.Tags{"pass": "p1"}))
	require.Equal(t, "check -latest -tag pass=p1", f.calls()[2])
}
