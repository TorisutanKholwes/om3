package plakar

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/backup"
)

const (
	id1 = "4c1f9f4a2b7d3e9a8c6b5f4e3d2c1b0a99887766554433221100ffeeddccbbaa"
	id2 = "8d2e0a5b3c8e4f0b9d7c6a5f4e3d2c1b0a99887766554433221100ffeeddccbb"
)

func TestParseListLine(t *testing.T) {
	t.Run("with tags", func(t *testing.T) {
		s, err := parseListLine("2026-10-14T08:00:00Z " + id1 + "   1.2 MiB        3s /srv/app1 tags=node=n1,path=test/svc/app1,rid=fs#1")
		require.NoError(t, err)
		require.Equal(t, backup.SnapshotID(id1), s.ID)
		require.Equal(t, time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC), s.Time)
		require.Equal(t, "/srv/app1", s.Root)
		require.Equal(t, backup.Tags{"node": "n1", "path": "test/svc/app1", "rid": "fs#1"}, s.Tags)
	})
	t.Run("without tags", func(t *testing.T) {
		s, err := parseListLine("2026-10-14T08:00:00Z " + id1 + "       0 B        0s /")
		require.NoError(t, err)
		require.Equal(t, "/", s.Root)
		require.Empty(t, s.Tags)
	})
	t.Run("root with a space", func(t *testing.T) {
		s, err := parseListLine("2026-10-14T08:00:00Z " + id1 + "    12 KiB      1m2s /srv/my app tags=a=b")
		require.NoError(t, err)
		require.Equal(t, "/srv/my app", s.Root)
	})
	for _, line := range []string{
		"",
		"plakar: could not open repository",
		"2026-10-14T08:00:00Z nothex 1 B 0s /",
		"2026-10-14T08:00:00Z " + id1,
	} {
		t.Run("invalid "+line, func(t *testing.T) {
			_, err := parseListLine(line)
			require.Error(t, err)
		})
	}
}

func TestList(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	f.answer("ls", ""+
		"2026-10-14T09:00:00Z "+id2+"   1.2 MiB        3s /srv/app1 tags=path=test/svc/app1,rid=fs#1\n"+
		"a warning plakar printed\n"+
		"2026-10-14T08:00:00Z "+id1+"   1.1 MiB        3s /srv/app1 tags=path=test/svc/app1,rid=fs#1\n", 0)

	l, err := s.List(context.Background(), backup.Tags{"path": "test/svc/app1", "rid": "fs#1"})
	require.NoError(t, err)
	require.Len(t, l, 2)
	require.Equal(t, backup.SnapshotID(id2), l[0].ID)
	require.Equal(t, []string{"ls -uuid -tags -tag path=test/svc/app1,rid=fs#1"}, f.calls())

	latest, err := s.latest(context.Background(), backup.Tags{"path": "test/svc/app1"})
	require.NoError(t, err)
	require.Equal(t, backup.SnapshotID(id2), latest.ID)
	require.Equal(t, "ls -uuid -tags -latest -tag path=test/svc/app1", f.calls()[1])
}

func TestListEmpty(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	l, err := s.List(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, l)
	require.Equal(t, []string{"ls -uuid -tags"}, f.calls())

	_, err = s.latest(context.Background(), backup.Tags{"path": "x"})
	require.ErrorIs(t, err, backup.ErrNoSnapshot)
}

func TestListRejectsInvalidTags(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	_, err := s.List(context.Background(), backup.Tags{"path": "a,b"})
	require.Error(t, err)
	require.Empty(t, f.calls())
}
