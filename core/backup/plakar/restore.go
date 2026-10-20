package plakar

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opensvc/om3/v3/core/backup"
)

// Restore restores the snapshot id in the dst directory.
//
// The files land directly under dst, without the root of the snapshot:
// the snapshot of /srv/app1 restored to /tmp/restore puts /srv/app1/a in
// /tmp/restore/a.
func (t *T) Restore(ctx context.Context, id backup.SnapshotID, dst string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if !filepath.IsAbs(dst) {
		return fmt.Errorf("store %s: restore to %s: not an absolute path", t.name, dst)
	}
	if err := os.MkdirAll(dst, 0700); err != nil {
		return fmt.Errorf("store %s: restore: %w", t.name, err)
	}
	args := []string{"restore", "-to", filepath.Clean(dst), string(id)}
	if err := t.run(ctx, call{args: args, quiet: true}); err != nil {
		return fmt.Errorf("store %s: restore %s to %s: %w", t.name, id, dst, err)
	}
	t.log.Infof("snapshot %s restored to %s", id, dst)
	return nil
}

// RestoreStream writes the name file of the snapshot id, a snapshot made
// by BackupStream, to w.
//
// plakar prints the errors of a running command on its standard output,
// so on error w may have received an error message: a caller writing to a
// file writes to a temporary one, renamed in place on success only.
func (t *T) RestoreStream(ctx context.Context, id backup.SnapshotID, name string, w io.Writer) error {
	if err := validateID(id); err != nil {
		return err
	}
	name = strings.TrimPrefix(filepath.Clean("/"+name), "/")
	if name == "" {
		return fmt.Errorf("store %s: restore stream: empty name", t.name)
	}
	args := []string{"cat", string(id) + ":/" + name}
	if err := t.run(ctx, call{args: args, stdout: w}); err != nil {
		return fmt.Errorf("store %s: restore stream %s:%s: %w", t.name, id, name, err)
	}
	return nil
}

// validateID returns an error if id is not a snapshot id plakar accepts as
// an argument: an hexadecimal id or id prefix.
func validateID(id backup.SnapshotID) error {
	if !isHex(string(id)) {
		return fmt.Errorf("invalid snapshot id %q", id)
	}
	return nil
}
