package ressyncbackup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/status"
	"github.com/opensvc/om3/v3/core/statusbus"
	"github.com/opensvc/om3/v3/core/store"
	"github.com/opensvc/om3/v3/core/xconfig"
	_ "github.com/opensvc/om3/v3/drivers/resfsdir"
	"github.com/opensvc/om3/v3/testhelper"
	"github.com/opensvc/om3/v3/util/hostname"
)

type (
	// fakeStore records the calls of the driver.
	fakeStore struct {
		sync.Mutex
		keep       store.Retention
		backups    []store.Tags
		paths      []string
		streams    map[string]string
		prunes     []store.Tags
		checks     int
		backupErr  error
		snapshots  []store.Snapshot
		restores   map[store.SnapshotID]string
		restoreErr error
	}
)

func (f *fakeStore) Name() string          { return "lab" }
func (f *fakeStore) SetConfig(*xconfig.T)  {}
func (f *fakeStore) Keep() store.Retention { return f.keep }

func (f *fakeStore) Backup(_ context.Context, src string, tags store.Tags) (store.SnapshotID, error) {
	f.Lock()
	defer f.Unlock()
	if f.backupErr != nil {
		return "", f.backupErr
	}
	f.backups = append(f.backups, tags)
	f.paths = append(f.paths, src)
	return "aa", nil
}

func (f *fakeStore) BackupStream(_ context.Context, r io.Reader, name string, tags store.Tags) (store.SnapshotID, error) {
	f.Lock()
	defer f.Unlock()
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if f.streams == nil {
		f.streams = make(map[string]string)
	}
	f.streams[name] = string(b)
	f.backups = append(f.backups, tags)
	return "bb", nil
}

func (f *fakeStore) RestoreStream(_ context.Context, id store.SnapshotID, name string, w io.Writer) error {
	_, err := io.WriteString(w, f.streams[name])
	return err
}

func (f *fakeStore) List(_ context.Context, filter store.Tags) ([]store.Snapshot, error) {
	l := make([]store.Snapshot, 0)
	for _, s := range f.snapshots {
		if s.Tags.Match(filter) {
			l = append(l, s)
		}
	}
	return l, nil
}

func (f *fakeStore) Restore(_ context.Context, id store.SnapshotID, dst string) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	if f.restores == nil {
		f.restores = make(map[store.SnapshotID]string)
	}
	f.restores[id] = dst
	return nil
}

func (f *fakeStore) Prune(_ context.Context, filter store.Tags, keep store.Retention) error {
	f.prunes = append(f.prunes, filter)
	return nil
}

func (f *fakeStore) Check(context.Context) error {
	f.checks++
	return nil
}

// setup returns the sync#1 resource of a test/svc/app1 service having a fs#1
// directory, and its context with the fs#1 status up.
func setup(t *testing.T, extra string) (*T, *fakeStore, context.Context, naming.Path) {
	t.Helper()
	testhelper.Setup(t)
	p := naming.Path{Namespace: "test", Kind: naming.KindSvc, Name: "app1"}
	dir := t.TempDir()
	conf := "[fs#1]\ntype = directory\npath = " + dir + "\n\n[sync#1]\ntype = backup\nstore = lab\n" + extra
	require.NoError(t, os.MkdirAll(filepath.Dir(p.ConfigFile()), 0700))
	require.NoError(t, os.WriteFile(p.ConfigFile(), []byte(conf), 0600))
	o, err := object.NewSvc(p)
	require.NoError(t, err)
	var r *T
	for _, rs := range o.Resources() {
		if rs.RID() == "sync#1" {
			r = rs.(*T)
		}
	}
	require.NotNil(t, r)
	st := &fakeStore{}
	r.openStore = func() (store.Store, error) { return st, nil }
	ctx, stop := statusbus.WithContext(context.Background(), p)
	t.Cleanup(stop)
	statusbus.FromContext(ctx).Post("fs#1", status.Up, false)
	return r, st, ctx, p
}

func TestUpdate(t *testing.T) {
	r, st, ctx, p := setup(t, "keep = 7d\nmax_delay = 1h\n")
	require.NoError(t, r.Update(ctx))

	require.Len(t, st.backups, 2)
	fsTags, configTags := st.backups[0], st.backups[1]
	require.Equal(t, "test/svc/app1", fsTags[store.TagPath])
	require.Equal(t, "fs#1", fsTags[store.TagRID])
	require.Equal(t, hostname.Hostname(), fsTags[store.TagNode])
	require.Equal(t, "sync#1", fsTags[store.TagSyncRID])
	require.NotEmpty(t, fsTags[store.TagPass])
	require.NoError(t, fsTags.Validate())
	require.Equal(t, "config", configTags[store.TagRID])
	require.Equal(t, fsTags[store.TagPass], configTags[store.TagPass])

	b, err := os.ReadFile(p.ConfigFile())
	require.NoError(t, err)
	require.Equal(t, string(b), st.streams["test/svc/app1.conf"])

	require.Equal(t, []store.Tags{
		{"path": "test/svc/app1", "rid": "fs#1"},
		{"path": "test/svc/app1", "rid": "config"},
	}, st.prunes)
	require.Equal(t, status.Up, r.Status(ctx))
}

func TestUpdateWithoutKeepDoesNotPrune(t *testing.T) {
	r, st, ctx, _ := setup(t, "")
	require.NoError(t, r.Update(ctx))
	require.Empty(t, st.prunes)
}

func TestUpdateUsesTheStoreKeep(t *testing.T) {
	r, st, ctx, _ := setup(t, "")
	st.keep = store.Retention{Days: 1}
	require.NoError(t, r.Update(ctx))
	require.Len(t, st.prunes, 2)
}

func TestUpdateFailure(t *testing.T) {
	r, st, ctx, _ := setup(t, "max_delay = 1h\n")
	st.backupErr = errors.New("store unreachable")
	require.ErrorContains(t, r.Update(ctx), "store unreachable")
	// no last sync time written: never synced
	require.NotEqual(t, status.Up, r.Status(ctx))
}

func TestUpdateRefusedOnPassiveInstance(t *testing.T) {
	r, st, ctx, _ := setup(t, "")
	statusbus.FromContext(ctx).Post("fs#1", status.Down, false)
	require.ErrorContains(t, r.Update(ctx), "not sufficiently started")
	require.Empty(t, st.backups)
	require.Equal(t, status.NotApplicable, r.Status(ctx))
}

func TestUpdateExcludedResource(t *testing.T) {
	r, st, ctx, _ := setup(t, "exclude = fs#1\n")
	require.NoError(t, r.Update(ctx))
	require.Len(t, st.backups, 1)
	require.Equal(t, "config", st.backups[0][store.TagRID])
}

func TestUpdateUnknownRID(t *testing.T) {
	r, _, ctx, _ := setup(t, "rids = fs#9\n")
	require.ErrorContains(t, r.Update(ctx), "fs#9")
}

func TestIsAvailable(t *testing.T) {
	require.True(t, isAvailable(status.Up))
	require.True(t, isAvailable(status.StandbyUp))
	require.True(t, isAvailable(status.NotApplicable))
	require.False(t, isAvailable(status.Down))
	require.False(t, isAvailable(status.Warn))
	require.False(t, isAvailable(status.Undef))
}

func TestFull(t *testing.T) {
	r, st, ctx, _ := setup(t, "")
	require.NoError(t, r.Full(ctx))
	require.Equal(t, 1, st.checks)
}
