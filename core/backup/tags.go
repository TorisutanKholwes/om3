package backup

import (
	"fmt"
	"slices"
	"strings"
)

type (
	// Tags are the search key of the snapshots. The framework gives them to
	// each Backup call, and filters List, Prune and Check with them, several
	// tags forming a conjunction.
	//
	// To confirm with OpenSVC: the syntax is "key=value", and a value
	// contains no comma, as the tags are passed to plakar as a single
	// comma separated list, and no space, as plakar lists them in a space
	// separated table.
	Tags map[string]string
)

const (
	// TagPath is the tag of the path of the object backed up.
	TagPath = "path"

	// TagRID is the tag of the resource backed up, or "config" for the
	// configuration of the service, or "ref" for a referenced key.
	TagRID = "rid"

	// TagNode is the tag of the node the backup ran on. It is informative
	// and never used as a restore filter: after a failover, the node now
	// active must find the snapshots made by the other one.
	TagNode = "node"

	// TagPass is the tag of the identifier of the pass, shared by all the
	// snapshots of one "sync update".
	TagPass = "pass"

	// TagSyncRID is the tag of the sync resource running the pass.
	TagSyncRID = "sync_rid"

	// TagRef is the tag of the "<path>:<key>" referenced key backed up.
	TagRef = "ref"
)

// ParseTags parses a "key=value,key=value" list.
func ParseTags(s string) (Tags, error) {
	tags := make(Tags)
	if s == "" {
		return tags, nil
	}
	for _, kv := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid tag %q: must be key=value", kv)
		}
		tags[k] = v
	}
	return tags, nil
}

// Validate returns an error if a tag can not be passed to a store as a
// member of a comma separated list.
func (t Tags) Validate() error {
	for k, v := range t {
		switch {
		case k == "":
			return fmt.Errorf("invalid tag: empty key")
		case strings.ContainsAny(k, ",= \t\n"):
			return fmt.Errorf("invalid tag key %q: must not contain a comma, an equal sign or a space", k)
		case strings.ContainsAny(v, ", \t\n"):
			return fmt.Errorf("invalid tag %s value %q: must not contain a comma or a space", k, v)
		}
	}
	return nil
}

// Keys returns the sorted keys of the tags.
func (t Tags) Keys() []string {
	l := make([]string, 0, len(t))
	for k := range t {
		l = append(l, k)
	}
	slices.Sort(l)
	return l
}

// String returns the "key=value,key=value" list, sorted by key so the same
// tags always make the same string.
func (t Tags) String() string {
	l := make([]string, 0, len(t))
	for _, k := range t.Keys() {
		l = append(l, k+"="+t[k])
	}
	return strings.Join(l, ",")
}

// Merge returns a copy of the tags with the other tags added, the other ones
// winning on a duplicate key.
func (t Tags) Merge(other Tags) Tags {
	m := make(Tags, len(t)+len(other))
	for k, v := range t {
		m[k] = v
	}
	for k, v := range other {
		m[k] = v
	}
	return m
}

// Match returns true if the tags have all the filter tags.
func (t Tags) Match(filter Tags) bool {
	for k, v := range filter {
		if w, ok := t[k]; !ok || w != v {
			return false
		}
	}
	return true
}
