// Package backup is the store side of the om3 backup framework: the Store
// interface a store driver implements, and the types the framework and the
// drivers exchange.
//
// The interface is the one of the OpenSVC design "om3 svc backup and
// restore". Until the framework is published, this package is a provisional
// copy of it, to be aligned on the published one. The points still to be
// confirmed with OpenSVC are marked "to confirm with OpenSVC".
package backup

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/opensvc/om3/v3/core/xconfig"
)

type (
	// SnapshotID identifies a snapshot in a store.
	SnapshotID string

	// Snapshot is a complete and independent backup of one path or one
	// stream, as listed by a store.
	Snapshot struct {
		ID   SnapshotID `json:"id"`
		Time time.Time  `json:"time"`
		Root string     `json:"root"`
		Tags Tags       `json:"tags"`
	}

	// Store is the interface a store driver implements.
	//
	// The framework decides what is backed up, when, and with which tags.
	// The driver only receives a path or a stream, and the tags to set.
	Store interface {
		Name() string
		SetConfig(*xconfig.T)
		Backup(ctx context.Context, src string, tags Tags) (SnapshotID, error)
		List(ctx context.Context, filter Tags) ([]Snapshot, error)
		Restore(ctx context.Context, id SnapshotID, dst string) error
		Prune(ctx context.Context, filter Tags, keep Retention) error
		Check(ctx context.Context) error
	}

	// StreamStore is implemented by the stores able to back up a stream,
	// and to read a single key of a snapshot back to a stream.
	//
	// To confirm with OpenSVC: the design gives Backup a path, but the
	// service configuration and the referenced sec and cfg keys are
	// streams.
	StreamStore interface {
		BackupStream(ctx context.Context, r io.Reader, name string, tags Tags) (SnapshotID, error)
		RestoreStream(ctx context.Context, id SnapshotID, name string, w io.Writer) error
	}

	// FilteredChecker is implemented by the stores able to check only the
	// latest snapshot matching a filter.
	//
	// To confirm with OpenSVC: the scope of Check is the whole store in the
	// design, but "sync full" may only want the snapshots of its pass.
	FilteredChecker interface {
		CheckFiltered(ctx context.Context, filter Tags) error
	}

	// namer is implemented by the stores knowing the name of the section
	// declaring them.
	namer interface {
		SetName(string)
	}
)

var (
	// ErrNoSnapshot is returned when no snapshot matches a filter.
	ErrNoSnapshot = errors.New("no snapshot found")
)

// Latest returns the most recent snapshot of the list, and false if the list
// is empty.
func Latest(l []Snapshot) (Snapshot, bool) {
	var (
		latest Snapshot
		found  bool
	)
	for _, s := range l {
		if !found || s.Time.After(latest.Time) {
			latest = s
			found = true
		}
	}
	return latest, found
}
