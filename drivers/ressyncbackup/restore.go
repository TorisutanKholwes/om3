package ressyncbackup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/opensvc/om3/v3/core/actioncontext"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/resource"
	"github.com/opensvc/om3/v3/core/store"
)

// Restore restores the snapshots of the object resources, and with the
// options, of its configuration and referenced keys.
//
// Each resource is restored from its latest snapshot, whichever node made
// it, so the node active after a failover restores what the other one
// backed up, or from the snapshot of the --snapshot pass.
//
// In place, a resource is restored in its mount point, which must be up.
// With --into, each resource is restored in a sub-directory named after
// its rid.
func (t *T) Restore(ctx context.Context, opts resource.RestoreOptions) error {
	disable := actioncontext.IsLockDisabled(ctx)
	timeout := actioncontext.LockTimeout(ctx)
	unlock, err := t.Lock(disable, timeout, lockName)
	if err != nil {
		return err
	}
	defer unlock()

	if opts.Into != "" && !filepath.IsAbs(opts.Into) {
		return fmt.Errorf("--into %s: not an absolute path", opts.Into)
	}
	sources, err := t.restoreSources(ctx, opts)
	if err != nil {
		return err
	}
	s, err := t.store()
	if err != nil {
		return err
	}
	var errs error
	for _, src := range sources {
		if err := t.restoreSource(ctx, s, opts, src); err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", src.rid, err))
		}
	}
	if opts.Config {
		if err := t.restoreConfig(ctx, s, opts); err != nil {
			errs = errors.Join(errs, fmt.Errorf("config: %w", err))
		}
	}
	if opts.Refs {
		if err := t.restoreRefs(ctx, s, opts); err != nil {
			errs = errors.Join(errs, fmt.Errorf("refs: %w", err))
		}
	}
	return errs
}

// restoreFilter returns the tags selecting the snapshots of rid to restore
// from: all the ones of the object resource, or the one of the pass.
func (t *T) restoreFilter(rid, passID string) store.Tags {
	filter := t.objectFilter(rid)
	if passID != "" {
		filter[store.TagPass] = passID
	}
	return filter
}

// latest returns the latest snapshot matching the filter.
func latest(ctx context.Context, s store.Store, filter store.Tags) (store.Snapshot, error) {
	l, err := s.List(ctx, filter)
	if err != nil {
		return store.Snapshot{}, err
	}
	snap, ok := store.Latest(l)
	if !ok {
		return store.Snapshot{}, fmt.Errorf("%w matching %s", store.ErrNoSnapshot, filter)
	}
	return snap, nil
}

func (t *T) restoreSource(ctx context.Context, s store.Store, opts resource.RestoreOptions, src source) error {
	snap, err := latest(ctx, s, t.restoreFilter(src.rid, opts.Snapshot))
	if err != nil {
		return err
	}
	dst := src.path
	if opts.Into != "" {
		dst = filepath.Join(opts.Into, src.rid)
	}
	t.Log().Infof("restore %s from snapshot %s of pass %s made on %s to %s", src.rid, snap.ID, snap.Tags[store.TagPass], snap.Tags[store.TagNode], dst)
	return s.Restore(ctx, snap.ID, dst)
}

// restoreSources returns the resources to restore: the ones the --rid
// selection names, or else the ones the resource backs up.
//
// A resource restored in place must expose a mount point and be available,
// so the files never land in the directory a fs is not mounted on.
func (t *T) restoreSources(ctx context.Context, opts resource.RestoreOptions) ([]source, error) {
	byRID := make(map[string]resource.Driver)
	for _, r := range t.GetObjectDriver().ResourcesByDrivergroups(backedUpGroups) {
		byRID[r.RID()] = r
	}
	rids := opts.RIDs
	if len(rids) == 0 {
		if opts.Config || opts.Refs {
			// only the configuration or the references
			return nil, nil
		}
		rids = t.RIDs
	}
	if len(rids) == 0 {
		for rid := range byRID {
			if !slices.Contains(t.Exclude, rid) {
				rids = append(rids, rid)
			}
		}
		slices.Sort(rids)
	}
	l := make([]source, 0, len(rids))
	for _, rid := range rids {
		r, ok := byRID[rid]
		if !ok {
			if opts.Into != "" {
				// a resource removed from the configuration can
				// still be restored aside
				l = append(l, source{rid: rid})
				continue
			}
			return nil, fmt.Errorf("%s is not a fs or volume resource of the object", rid)
		}
		if opts.Into != "" {
			l = append(l, source{rid: rid})
			continue
		}
		h, ok := r.(header)
		if !ok || h.Head() == "" {
			return nil, fmt.Errorf("%s exposes no mount point to restore in place", rid)
		}
		if st := resource.EvalStatus(ctx, r); !isAvailable(st) {
			return nil, fmt.Errorf("%s status is %s: refuse to restore in place", rid, st)
		}
		l = append(l, source{rid: rid, path: h.Head()})
	}
	if len(l) == 0 {
		return nil, fmt.Errorf("no resource to restore")
	}
	return l, nil
}

// restoreConfig restores the configuration of the object, in place or in
// the --into directory.
func (t *T) restoreConfig(ctx context.Context, s store.Store, opts resource.RestoreOptions) error {
	ss, ok := s.(store.StreamStore)
	if !ok {
		return fmt.Errorf("the store %s can not restore a stream", t.Store)
	}
	snap, err := latest(ctx, s, t.restoreFilter(configRID, opts.Snapshot))
	if err != nil {
		return err
	}
	var dst string
	if opts.Into != "" {
		dst = filepath.Join(opts.Into, t.configName())
	} else if o, ok := t.GetObject().(configFiler); ok {
		dst = o.ConfigFile()
	} else {
		return fmt.Errorf("the object has no configuration file")
	}
	var b bytes.Buffer
	if err := ss.RestoreStream(ctx, snap.ID, t.configName(), &b); err != nil {
		return err
	}
	t.Log().Infof("restore the configuration from snapshot %s to %s", snap.ID, dst)
	return writeFileAtomic(dst, b.Bytes())
}

// restoreRefs restores the referenced keys, in place in their sec and cfg
// objects, or as files in the --into directory.
func (t *T) restoreRefs(ctx context.Context, s store.Store, opts resource.RestoreOptions) error {
	ss, ok := s.(store.StreamStore)
	if !ok {
		return fmt.Errorf("the store %s can not restore a stream", t.Store)
	}
	l, err := s.List(ctx, t.restoreFilter(refRID, opts.Snapshot))
	if err != nil {
		return err
	}
	// the latest snapshot of each key
	byRef := make(map[string]store.Snapshot)
	for _, snap := range l {
		k := snap.Tags[store.TagRef]
		if prev, ok := byRef[k]; !ok || snap.Time.After(prev.Time) {
			byRef[k] = snap
		}
	}
	if len(byRef) == 0 {
		return fmt.Errorf("%w matching %s", store.ErrNoSnapshot, t.restoreFilter(refRID, opts.Snapshot))
	}
	var errs error
	for k, snap := range byRef {
		r, err := parseRef(k)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		var b bytes.Buffer
		if err := ss.RestoreStream(ctx, snap.ID, refName(r), &b); err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", r, err))
			continue
		}
		if opts.Into != "" {
			err = writeFileAtomic(filepath.Join(opts.Into, refName(r)), b.Bytes())
		} else {
			err = t.restoreRefInPlace(r, b.Bytes())
		}
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", r, err))
			continue
		}
		t.Log().Infof("restored the key %s from snapshot %s", r, snap.ID)
	}
	return errs
}

func (t *T) restoreRefInPlace(r ref, b []byte) error {
	ds, err := object.NewDataStore(r.path)
	if err != nil {
		return err
	}
	if !ds.Allow(t.Path.Namespace) {
		return fmt.Errorf("the %s namespace is not allowed to access %s keys", t.Path.Namespace, r.path)
	}
	if ds.HasKey(r.key) {
		return ds.ChangeKey(r.key, b)
	}
	return ds.AddKey(r.key, b)
}

// parseRef parses a "<path>:<key>" ref tag.
func parseRef(s string) (ref, error) {
	ps, k, ok := strings.Cut(s, ":")
	if !ok || k == "" {
		return ref{}, fmt.Errorf("invalid ref tag %q", s)
	}
	p, err := naming.ParsePath(ps)
	if err != nil {
		return ref{}, fmt.Errorf("invalid ref tag %q: %w", s, err)
	}
	return ref{path: p, key: k}, nil
}

// writeFileAtomic writes a 0600 file through a temporary file renamed in
// place, so a failed restore never leaves a partial file.
func writeFileAtomic(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
