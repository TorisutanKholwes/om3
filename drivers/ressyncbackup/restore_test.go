package ressyncbackup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/backup"
	"github.com/opensvc/om3/v3/core/resource"
)

func snapshots(now time.Time) []backup.Snapshot {
	tags := func(rid, pass, node string) backup.Tags {
		return backup.Tags{"path": "test/svc/app1", "rid": rid, "pass": pass, "node": node}
	}
	return []backup.Snapshot{
		{ID: "a1", Time: now.Add(-2 * time.Hour), Tags: tags("fs#1", "p1", "n1")},
		{ID: "a2", Time: now.Add(-time.Hour), Tags: tags("fs#1", "p2", "n2")},
		{ID: "c1", Time: now.Add(-2 * time.Hour), Tags: tags("config", "p1", "n1")},
		{ID: "c2", Time: now.Add(-time.Hour), Tags: tags("config", "p2", "n2")},
		{ID: "o1", Time: now, Tags: backup.Tags{"path": "test/svc/other", "rid": "fs#1", "pass": "p3"}},
	}
}

func TestRestoreLatestInPlace(t *testing.T) {
	r, store, ctx, _ := setup(t, "")
	store.snapshots = snapshots(time.Now())
	require.NoError(t, r.Restore(ctx, resource.RestoreOptions{}))
	// the latest snapshot of the object resource, whichever node made it
	require.Len(t, store.restores, 1)
	require.Contains(t, store.restores, backup.SnapshotID("a2"))
	require.Equal(t, r.GetObjectDriver().ResourcesByDrivergroups(backedUpGroups)[0].(header).Head(), store.restores["a2"])
}

func TestRestorePassInto(t *testing.T) {
	r, store, ctx, _ := setup(t, "")
	store.snapshots = snapshots(time.Now())
	into := t.TempDir()
	require.NoError(t, r.Restore(ctx, resource.RestoreOptions{Into: into, Snapshot: "p1", RIDs: []string{"fs#1"}}))
	require.Equal(t, map[backup.SnapshotID]string{"a1": filepath.Join(into, "fs#1")}, store.restores)
}

func TestRestoreConfigInto(t *testing.T) {
	r, store, ctx, _ := setup(t, "")
	store.snapshots = snapshots(time.Now())
	store.streams = map[string]string{"test/svc/app1.conf": "[DEFAULT]\n"}
	into := t.TempDir()
	require.NoError(t, r.Restore(ctx, resource.RestoreOptions{Into: into, Config: true}))
	// only the configuration
	require.Empty(t, store.restores)
	b, err := os.ReadFile(filepath.Join(into, "test/svc/app1.conf"))
	require.NoError(t, err)
	require.Equal(t, "[DEFAULT]\n", string(b))
}

func TestRestoreNoSnapshot(t *testing.T) {
	r, store, ctx, _ := setup(t, "")
	store.snapshots = snapshots(time.Now())
	require.ErrorIs(t, r.Restore(ctx, resource.RestoreOptions{Into: t.TempDir(), Snapshot: "p9"}), backup.ErrNoSnapshot)
	require.Empty(t, store.restores)
}

func TestRestoreRefusesRelativeInto(t *testing.T) {
	r, _, ctx, _ := setup(t, "")
	require.Error(t, r.Restore(ctx, resource.RestoreOptions{Into: "relative"}))
}

func TestRestoreUnknownRIDInPlace(t *testing.T) {
	r, _, ctx, _ := setup(t, "")
	require.ErrorContains(t, r.Restore(ctx, resource.RestoreOptions{RIDs: []string{"fs#9"}}), "fs#9")
}

func TestRestoreRefsInto(t *testing.T) {
	r, store, ctx, _ := setup(t, "")
	now := time.Now()
	store.snapshots = []backup.Snapshot{
		{ID: "r1", Time: now.Add(-time.Hour), Tags: backup.Tags{"path": "test/svc/app1", "rid": "ref", "ref": "test/cfg/s1:a"}},
		{ID: "r2", Time: now, Tags: backup.Tags{"path": "test/svc/app1", "rid": "ref", "ref": "test/cfg/s1:a"}},
	}
	store.streams = map[string]string{"refs/test/cfg/s1/a": "va"}
	into := t.TempDir()
	require.NoError(t, r.Restore(ctx, resource.RestoreOptions{Into: into, Refs: true}))
	b, err := os.ReadFile(filepath.Join(into, "refs/test/cfg/s1/a"))
	require.NoError(t, err)
	require.Equal(t, "va", string(b))
}
