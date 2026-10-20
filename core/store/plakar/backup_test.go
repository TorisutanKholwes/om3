package plakar

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/store"
)

func TestBackup(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	src := t.TempDir()
	tags := store.Tags{"path": "test/svc/app1", "rid": "fs#1", "pass": "p1"}
	f.answer("ls", "2026-10-14T09:00:00Z "+id1+"   1.2 MiB        3s "+src+" tags="+tags.String()+"\n", 0)
	f.answer("backup", "4c1f9f4a: backup completed without errors in 3s\n", 0)

	id, err := s.Backup(context.Background(), src, tags)
	require.NoError(t, err)
	require.Equal(t, store.SnapshotID(id1), id)
	require.Equal(t, []string{
		"backup -no-progress -tag pass=p1,path=test/svc/app1,rid=fs#1 -name test/svc/app1 " + src,
		"ls -uuid -tags -latest -tag pass=p1,path=test/svc/app1,rid=fs#1",
	}, f.calls())
	require.Contains(t, f.rawCalls()[0], "-quiet at @lab backup")
}

func TestBackupErrors(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	_, err := s.Backup(context.Background(), "relative/path", nil)
	require.Error(t, err)
	_, err = s.Backup(context.Background(), "/does/not/exist", nil)
	require.Error(t, err)
	_, err = s.Backup(context.Background(), t.TempDir(), store.Tags{"a": "b,c"})
	require.Error(t, err)
	require.Empty(t, f.calls())

	f.answer("backup", "plakar: backup: permission denied\n", 1)
	_, err = s.Backup(context.Background(), t.TempDir(), nil)
	require.ErrorContains(t, err, "permission denied")
}

func TestBackupStream(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	f.capture("backup")
	tags := store.Tags{"path": "test/svc/app1", "rid": "config"}
	f.answer("ls", "2026-10-14T09:00:00Z "+id1+"   1 KiB        0s / tags="+tags.String()+"\n", 0)

	id, err := s.BackupStream(context.Background(), strings.NewReader("[DEFAULT]\nid = x\n"), "test/svc/app1.conf", tags)
	require.NoError(t, err)
	require.Equal(t, store.SnapshotID(id1), id)
	require.Equal(t, "backup -no-progress -tag path=test/svc/app1,rid=config -name test/svc/app1 stdin:///test/svc/app1.conf", f.calls()[0])
	require.Equal(t, "[DEFAULT]\nid = x\n", f.stdin("backup"))
}

func TestBackupStreamNotRetried(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	f.answer("backup", "plakar: could not resolve repository: @lab\n", 1)
	_, err := s.BackupStream(context.Background(), strings.NewReader("x"), "x", nil)
	require.Error(t, err)
	require.Len(t, f.calls(), 1)
}
