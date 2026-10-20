package plakar

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opensvc/om3/v3/core/store"
)

// Backup makes a snapshot of the src directory, with the tags, and returns
// its id.
//
// plakar prints only a short id when a backup completes, so the full id is
// the one of the latest snapshot having the tags, which include the
// identifier of the pass.
func (t *T) Backup(ctx context.Context, src string, tags store.Tags) (store.SnapshotID, error) {
	if err := tags.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(src) {
		return "", fmt.Errorf("store %s: backup %s: not an absolute path", t.name, src)
	}
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("store %s: backup: %w", t.name, err)
	}
	args := t.backupArgs(tags)
	args = append(args, filepath.Clean(src))
	if err := t.run(ctx, call{args: args, quiet: true}); err != nil {
		return "", fmt.Errorf("store %s: backup %s: %w", t.name, src, err)
	}
	return t.backedUpID(ctx, tags)
}

// BackupStream makes a snapshot of the r stream, stored as the name file,
// with the tags, and returns its id.
//
// The stream is passed to the plakar stdin importer, so its bytes do not
// touch the disk of the node.
func (t *T) BackupStream(ctx context.Context, r io.Reader, name string, tags store.Tags) (store.SnapshotID, error) {
	if err := tags.Validate(); err != nil {
		return "", err
	}
	name = strings.TrimPrefix(filepath.Clean("/"+name), "/")
	if name == "" {
		return "", fmt.Errorf("store %s: backup stream: empty name", t.name)
	}
	args := t.backupArgs(tags)
	args = append(args, "stdin:///"+name)
	if err := t.run(ctx, call{args: args, stdin: r, quiet: true}); err != nil {
		return "", fmt.Errorf("store %s: backup stream %s: %w", t.name, name, err)
	}
	return t.backedUpID(ctx, tags)
}

// backupArgs returns the backup command arguments preceding the source.
//
// The -name is the path of the object, which makes the snapshots readable
// in the plakar interface without knowing the tags.
func (t *T) backupArgs(tags store.Tags) []string {
	args := []string{"backup", "-no-progress"}
	if len(tags) > 0 {
		args = append(args, "-tag", tags.String())
	}
	if name := tags[store.TagPath]; name != "" {
		args = append(args, "-name", name)
	}
	return args
}

func (t *T) backedUpID(ctx context.Context, tags store.Tags) (store.SnapshotID, error) {
	s, err := t.latest(ctx, tags)
	if err != nil {
		return "", fmt.Errorf("store %s: backup done, but its snapshot is not found: %w", t.name, err)
	}
	t.log.Infof("snapshot %s done", s.ID)
	return s.ID, nil
}
