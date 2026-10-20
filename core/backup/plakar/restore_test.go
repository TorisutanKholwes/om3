package plakar

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestore(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	dst := filepath.Join(t.TempDir(), "restore")
	require.NoError(t, s.Restore(context.Background(), id1, dst))
	require.Equal(t, []string{"restore -to " + dst + " " + id1}, f.calls())
	info, err := os.Stat(dst)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestRestoreErrors(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.Error(t, s.Restore(context.Background(), "", t.TempDir()))
	require.Error(t, s.Restore(context.Background(), "-to", t.TempDir()))
	require.Error(t, s.Restore(context.Background(), id1, "relative"))
	require.Empty(t, f.calls())

	f.answer("restore", "restore: no snapshots found\n", 1)
	require.ErrorContains(t, s.Restore(context.Background(), id1, t.TempDir()), "no snapshots found")
}

func TestRestoreStream(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	f.answer("cat", "[DEFAULT]\nid = x\n", 0)
	var b bytes.Buffer
	require.NoError(t, s.RestoreStream(context.Background(), id1, "test/svc/app1.conf", &b))
	require.Equal(t, "[DEFAULT]\nid = x\n", b.String())
	require.Equal(t, []string{"cat " + id1 + ":/test/svc/app1.conf"}, f.calls())
}
