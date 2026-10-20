package ressyncstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opensvc/om3/v3/core/datarecv"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/store"
	"github.com/opensvc/om3/v3/core/xconfig"
	"github.com/opensvc/om3/v3/util/key"
)

type (
	configer interface {
		Config() *xconfig.T
	}

	// ref is a key of a referenced sec or cfg object.
	ref struct {
		path naming.Path
		key  string
	}
)

const (
	// refRID is the rid tag of the snapshots of the referenced keys.
	refRID = "ref"
)

var (
	// refOptions are the keywords whose words reference keys as
	// "<name>/<key>", and the kind of the object they name.
	refOptions = map[string]naming.Kind{
		"configs":             naming.KindCfg,
		"configs_environment": naming.KindCfg,
		"secrets":             naming.KindSec,
		"secrets_environment": naming.KindSec,
	}
)

func (t ref) String() string {
	return t.path.String() + ":" + t.key
}

// referencedPaths returns the paths of the sec and cfg objects the
// configuration references, sorted.
func referencedPaths(cfg *xconfig.T, namespace string) []naming.Path {
	m := make(map[string]naming.Path)
	add := func(p naming.Path) {
		if p.Kind != naming.KindSec && p.Kind != naming.KindCfg {
			return
		}
		m[p.String()] = p
	}
	for _, section := range cfg.SectionStrings() {
		for _, option := range cfg.Keys(section) {
			value := cfg.Get(key.New(section, option))
			words := strings.Fields(value)
			for i, word := range words {
				if word != "from" || i+1 >= len(words) {
					continue
				}
				if p, err := naming.ParsePathRel(words[i+1], namespace); err == nil {
					add(p)
				}
			}
			// the scoped keywords, as in "configs@node1"
			base, _, _ := strings.Cut(option, "@")
			kind, ok := refOptions[base]
			if !ok {
				continue
			}
			for _, word := range words {
				word = strings.Trim(word, `"'`)
				if _, after, ok := strings.Cut(word, "="); ok {
					word = after
				}
				name, _, ok := strings.Cut(word, "/")
				if !ok || name == "" || strings.ContainsAny(name, "{}*?[") {
					continue
				}
				if p, err := naming.NewPath(namespace, kind, name); err == nil {
					add(p)
				}
			}
		}
	}
	l := make([]naming.Path, 0, len(m))
	for _, p := range m {
		l = append(l, p)
	}
	slices.SortFunc(l, func(a, b naming.Path) int {
		return strings.Compare(a.String(), b.String())
	})
	return l
}

// backupRefs makes a snapshot of each key of the referenced sec and cfg
// objects, read as a stream, and returns the filters selecting them for the
// prune.
func (t *T) backupRefs(ctx context.Context, s store.Store, passID string) ([]store.Tags, error) {
	ss, ok := s.(store.StreamStore)
	if !ok {
		return nil, fmt.Errorf("the store %s can not back up a stream", t.Store)
	}
	o, ok := t.GetObject().(configer)
	if !ok {
		return nil, fmt.Errorf("the object has no configuration")
	}
	var (
		errs    error
		filters []store.Tags
	)
	for _, p := range referencedPaths(o.Config(), t.Path.Namespace) {
		refs, err := t.refKeys(p)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		for _, r := range refs {
			if err := t.backupRef(ctx, ss, passID, r); err != nil {
				errs = errors.Join(errs, fmt.Errorf("%s: %w", r, err))
				continue
			}
			filters = append(filters, t.refFilter(r))
		}
	}
	return filters, errs
}

// refKeys returns the keys of a referenced object, which the namespace of
// the service must be allowed to read.
func (t *T) refKeys(p naming.Path) ([]ref, error) {
	if !p.Exists() {
		return nil, fmt.Errorf("%s: the referenced object does not exist", p)
	}
	ds, err := object.NewDataStore(p, object.WithVolatile(true))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if !ds.Allow(t.Path.Namespace) {
		return nil, fmt.Errorf("the %s namespace is not allowed to access %s keys", t.Path.Namespace, p)
	}
	keys, err := ds.AllKeys()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	l := make([]ref, 0, len(keys))
	for _, k := range keys {
		l = append(l, ref{path: p, key: k})
	}
	return l, nil
}

func (t *T) backupRef(ctx context.Context, ss store.StreamStore, passID string, r ref) error {
	km := datarecv.KeyMeta{Path: r.path, Key: r.key, From: t.Path.Namespace}
	b, err := km.Decode()
	if err != nil {
		return err
	}
	tags := t.tags(passID, refRID)
	tags[store.TagRef] = r.String()
	if err := tags.Validate(); err != nil {
		return err
	}
	_, err = ss.BackupStream(ctx, bytes.NewReader(b), refName(r), tags)
	return err
}

// refFilter returns the tags selecting the snapshots of a referenced key.
func (t *T) refFilter(r ref) store.Tags {
	filter := t.objectFilter(refRID)
	filter[store.TagRef] = r.String()
	return filter
}

// refName is the name of a referenced key in its snapshot.
func refName(r ref) string {
	return "refs/" + r.path.String() + "/" + r.key
}
