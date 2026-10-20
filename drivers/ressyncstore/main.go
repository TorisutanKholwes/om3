// Package ressyncstore is the provisional sync resource of type store.
//
// It stands for the OpenSVC store framework until the framework is
// published, limited to calling the store driver: each "sync update" is a
// pass making one complete snapshot per selected fs or volume resource, plus
// one of the service configuration, all tagged with the identifier of the
// pass, then pruning them according to the keep retention.
//
// The driver decides when and what is backed up. The store driver only
// receives a path or a stream, and the tags to set.
package ressyncstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/opensvc/om3/v3/core/actioncontext"
	"github.com/opensvc/om3/v3/core/driver"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/provisioned"
	"github.com/opensvc/om3/v3/core/resource"
	"github.com/opensvc/om3/v3/core/status"
	"github.com/opensvc/om3/v3/core/statusbus"
	"github.com/opensvc/om3/v3/core/store"
	_ "github.com/opensvc/om3/v3/core/store/plakar"
	"github.com/opensvc/om3/v3/drivers/ressync"
	"github.com/opensvc/om3/v3/util/hostname"
	"github.com/opensvc/om3/v3/util/plog"
)

type (
	T struct {
		ressync.T
		Store   string   `json:"store"`
		RIDs    []string `json:"rids"`
		Exclude []string `json:"exclude"`
		Keep    string   `json:"keep"`

		IncludeRefs bool `json:"include_refs"`

		// openStore returns the store driver, newStore unless a test
		// replaces it.
		openStore func() (store.Store, error)
	}

	// source is a resource to back up.
	source struct {
		rid  string
		path string
	}

	header interface {
		Head() string
	}

	configFiler interface {
		ConfigFile() string
	}

	loggerSetter interface {
		SetLogger(*plog.Logger)
	}

	keeper interface {
		Keep() store.Retention
	}
)

const (
	lockName = "sync"

	// configRID is the rid tag of the snapshot of the service
	// configuration.
	configRID = "config"

	// passIDLayout is the layout of the identifier of a pass, the UTC time
	// it began at.
	passIDLayout = "20060102T150405Z"
)

var (
	backedUpGroups = []driver.Group{driver.GroupFS, driver.GroupVolume}
)

func New() resource.Driver {
	return &T{}
}

func (t *T) store() (store.Store, error) {
	if t.openStore != nil {
		return t.openStore()
	}
	return t.newStore()
}

// newStore returns the driver of the store the resource uses, configured
// from its [store#<name>] section of the cluster configuration.
func (t *T) newStore() (store.Store, error) {
	node, err := object.NewNode(object.WithVolatile(true))
	if err != nil {
		return nil, err
	}
	s, err := store.New(node.MergedConfig(), t.Store)
	if err != nil {
		return nil, err
	}
	if i, ok := s.(loggerSetter); ok {
		i.SetLogger(t.Log())
	}
	return s, nil
}

// retention returns the keep of the resource, or else the one of the store.
func (t *T) retention(s store.Store) (store.Retention, error) {
	if t.Keep != "" {
		return store.ParseRetention(t.Keep)
	}
	if i, ok := s.(keeper); ok {
		return i.Keep(), nil
	}
	return store.Retention{}, nil
}

func (t *T) ScheduleOptions() resource.ScheduleOptions {
	return resource.ScheduleOptions{
		Action: "sync_update",
		Option: "schedule",
		Base:   "",
	}
}

func (t *T) Provisioned(context.Context) (provisioned.T, error) {
	return provisioned.NotApplicable, nil
}

func (t *T) Running() (resource.RunningInfoList, error) {
	return t.RunningFromLock(lockName)
}

// Label returns the store name and the retention.
func (t *T) Label(_ context.Context) string {
	s := "to " + t.Store
	if t.Keep != "" {
		s += " keep " + t.Keep
	}
	return s
}

// Status is the freshness of the last pass made on this node.
//
// The store is shared, so a passive instance has nothing to say. Until its
// first pass, the node now active after a failover says no pass happened
// yet, as the other sync drivers do.
func (t *T) Status(ctx context.Context) status.T {
	if v, _ := t.IsInstanceSufficientlyStarted(ctx); !v {
		t.StatusLog().Info("instance not sufficiently started")
		return status.NotApplicable
	}
	return t.StatusLastSync([]string{hostname.Hostname()})
}

// Update runs a pass: one snapshot per selected resource and one of the
// service configuration, then the prune of their older snapshots.
func (t *T) Update(ctx context.Context) error {
	disable := actioncontext.IsLockDisabled(ctx)
	timeout := actioncontext.LockTimeout(ctx)
	unlock, err := t.Lock(disable, timeout, lockName)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = t.lockedPass(ctx)
	return err
}

// Full runs a pass, then checks the store.
//
// To confirm with OpenSVC: the scope of the check, the whole store in the
// design.
func (t *T) Full(ctx context.Context) error {
	disable := actioncontext.IsLockDisabled(ctx)
	timeout := actioncontext.LockTimeout(ctx)
	unlock, err := t.Lock(disable, timeout, lockName)
	if err != nil {
		return err
	}
	defer unlock()
	s, err := t.lockedPass(ctx)
	if err != nil {
		return err
	}
	t.Log().Infof("check the store %s", t.Store)
	return s.Check(ctx)
}

// lockedPass runs a pass and returns the store it used.
//
// The pass is refused on an instance not sufficiently started, so a passive
// node never overwrites the snapshots of the active one with its own empty
// or stale data. The last sync time is written only after a pass with at
// least one snapshot and no error.
func (t *T) lockedPass(ctx context.Context) (store.Store, error) {
	if v, rids := t.IsInstanceSufficientlyStarted(ctx); !v {
		return nil, fmt.Errorf("the instance is not sufficiently started (%s): refuse to back up", strings.Join(rids, ","))
	}
	sources, err := t.sources(ctx)
	if err != nil {
		return nil, err
	}
	s, err := t.store()
	if err != nil {
		return nil, err
	}
	keep, err := t.retention(s)
	if err != nil {
		return nil, fmt.Errorf("keep: %w", err)
	}
	passID := time.Now().UTC().Format(passIDLayout)
	t.Log().Infof("pass %s: back up %d resources and the configuration to the store %s", passID, len(sources), t.Store)

	var (
		errs  error
		done  int
		prune []store.Tags
	)
	for _, src := range sources {
		tags := t.tags(passID, src.rid)
		if _, err := s.Backup(ctx, src.path, tags); err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", src.rid, err))
			continue
		}
		done++
		prune = append(prune, t.objectFilter(src.rid))
	}
	if err := t.backupConfig(ctx, s, passID); err != nil {
		errs = errors.Join(errs, fmt.Errorf("config: %w", err))
	} else {
		done++
		prune = append(prune, t.objectFilter(configRID))
	}
	if t.IncludeRefs {
		filters, err := t.backupRefs(ctx, s, passID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("refs: %w", err))
		}
		done += len(filters)
		prune = append(prune, filters...)
	}
	if errs != nil {
		return nil, errs
	}
	if done == 0 {
		return nil, fmt.Errorf("pass %s: nothing backed up", passID)
	}
	if !keep.IsZero() {
		for _, filter := range prune {
			if err := s.Prune(ctx, filter, keep); err != nil {
				errs = errors.Join(errs, err)
			}
		}
		if errs != nil {
			return nil, errs
		}
	}
	if err := t.WriteLastSync(hostname.Hostname()); err != nil {
		return nil, err
	}
	t.Log().Infof("pass %s: %d snapshots done", passID, done)
	return s, nil
}

// tags returns the tags of the snapshot of rid made by the pass.
func (t *T) tags(passID, rid string) store.Tags {
	return store.Tags{
		store.TagPath:    t.Path.String(),
		store.TagRID:     rid,
		store.TagNode:    hostname.Hostname(),
		store.TagPass:    passID,
		store.TagSyncRID: t.RID(),
	}
}

// objectFilter returns the tags selecting the snapshots of rid, made by any
// node and any pass.
func (t *T) objectFilter(rid string) store.Tags {
	return store.Tags{
		store.TagPath: t.Path.String(),
		store.TagRID:  rid,
	}
}

// backupConfig makes the snapshot of the service configuration, read as a
// stream.
func (t *T) backupConfig(ctx context.Context, s store.Store, passID string) error {
	ss, ok := s.(store.StreamStore)
	if !ok {
		return fmt.Errorf("the store %s can not back up a stream", t.Store)
	}
	o, ok := t.GetObject().(configFiler)
	if !ok {
		return fmt.Errorf("the object has no configuration file")
	}
	b, err := os.ReadFile(o.ConfigFile())
	if err != nil {
		return err
	}
	_, err = ss.BackupStream(ctx, bytes.NewReader(b), t.configName(), t.tags(passID, configRID))
	return err
}

// configName is the name of the configuration file in its snapshot.
func (t *T) configName() string {
	return t.Path.String() + ".conf"
}

// isAvailable returns true if a resource status says its mount point holds
// its data: up, standby up, or n/a for a resource having nothing to mount,
// like a fs of type directory whose directory exists.
func isAvailable(st status.T) bool {
	switch st {
	case status.Up, status.StandbyUp, status.NotApplicable:
		return true
	default:
		return false
	}
}

// sources returns the resources to back up: the rids selection, or else the
// fs and volume resources exposing a mount point, less the excluded ones.
//
// A resource whose status is not available is skipped, its mount point
// being possibly empty, but a resource the rids keyword names is an error.
func (t *T) sources(ctx context.Context) ([]source, error) {
	sb := statusbus.FromContext(ctx)
	l := make([]source, 0)
	byRID := make(map[string]resource.Driver)
	for _, r := range t.GetObjectDriver().ResourcesByDrivergroups(backedUpGroups) {
		byRID[r.RID()] = r
	}
	rids := t.RIDs
	explicit := len(rids) > 0
	if !explicit {
		for rid := range byRID {
			rids = append(rids, rid)
		}
		slices.Sort(rids)
	}
	for _, rid := range rids {
		if slices.Contains(t.Exclude, rid) {
			continue
		}
		r, ok := byRID[rid]
		if !ok {
			return nil, fmt.Errorf("rids: %s is not a fs or volume resource of the object", rid)
		}
		h, ok := r.(header)
		if !ok || h.Head() == "" {
			if explicit {
				return nil, fmt.Errorf("rids: %s exposes no mount point", rid)
			}
			continue
		}
		if st := sb.Get(rid); !isAvailable(st) {
			if explicit {
				return nil, fmt.Errorf("rids: %s status is %s", rid, st)
			}
			t.Log().Infof("skip %s: status is %s", rid, st)
			continue
		}
		l = append(l, source{rid: rid, path: h.Head()})
	}
	return l, nil
}
