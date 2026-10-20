package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRetention(t *testing.T) {
	t.Run("days weeks months", func(t *testing.T) {
		r, err := ParseRetention("7d 4w 6m")
		require.NoError(t, err)
		require.Equal(t, Retention{Days: 7, Weeks: 4, Months: 6}, r)
		require.Equal(t, "7d 4w 6m", r.String())
	})
	t.Run("years", func(t *testing.T) {
		r, err := ParseRetention("2y")
		require.NoError(t, err)
		require.Equal(t, Retention{Years: 2}, r)
	})
	t.Run("empty is zero", func(t *testing.T) {
		r, err := ParseRetention("  ")
		require.NoError(t, err)
		require.True(t, r.IsZero())
	})
	for _, s := range []string{"7", "d", "0d", "-1d", "7x", "7d 3d", "1.5w"} {
		t.Run("invalid "+s, func(t *testing.T) {
			_, err := ParseRetention(s)
			require.Error(t, err)
		})
	}
}

func TestTags(t *testing.T) {
	t.Run("string is sorted", func(t *testing.T) {
		tags := Tags{"rid": "fs#1", "path": "test/svc/app1", "node": "n1"}
		require.Equal(t, "node=n1,path=test/svc/app1,rid=fs#1", tags.String())
	})
	t.Run("parse round trip", func(t *testing.T) {
		tags, err := ParseTags("node=n1,path=test/svc/app1,rid=fs#1")
		require.NoError(t, err)
		require.Equal(t, Tags{"rid": "fs#1", "path": "test/svc/app1", "node": "n1"}, tags)
	})
	t.Run("parse invalid", func(t *testing.T) {
		_, err := ParseTags("a=b,c")
		require.Error(t, err)
	})
	t.Run("validate rejects a comma in a value", func(t *testing.T) {
		require.Error(t, Tags{"a": "b,c"}.Validate())
		require.Error(t, Tags{"a,b": "c"}.Validate())
		require.Error(t, Tags{"a": "b c"}.Validate())
		require.Error(t, Tags{"": "c"}.Validate())
		require.NoError(t, Tags{"a": "b=c"}.Validate())
	})
	t.Run("match is a conjunction", func(t *testing.T) {
		tags := Tags{"path": "p", "rid": "fs#1"}
		require.True(t, tags.Match(Tags{"path": "p"}))
		require.True(t, tags.Match(Tags{"path": "p", "rid": "fs#1"}))
		require.False(t, tags.Match(Tags{"path": "p", "rid": "fs#2"}))
		require.False(t, tags.Match(Tags{"node": "n1"}))
	})
	t.Run("merge", func(t *testing.T) {
		require.Equal(t, Tags{"a": "1", "b": "3"}, Tags{"a": "1", "b": "2"}.Merge(Tags{"b": "3"}))
	})
}

func TestLatest(t *testing.T) {
	now := time.Now()
	_, ok := Latest(nil)
	require.False(t, ok)
	s, ok := Latest([]Snapshot{
		{ID: "a", Time: now.Add(-time.Hour)},
		{ID: "b", Time: now},
		{ID: "c", Time: now.Add(-2 * time.Hour)},
	})
	require.True(t, ok)
	require.Equal(t, SnapshotID("b"), s.ID)
}

func TestSectionName(t *testing.T) {
	require.Equal(t, "store#lab", SectionName("lab"))
	require.Equal(t, "store#lab", SectionName("store#lab"))
}
