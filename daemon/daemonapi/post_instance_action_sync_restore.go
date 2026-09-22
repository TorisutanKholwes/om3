package daemonapi

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/opensvc/om3/v3/core/client"
	"github.com/opensvc/om3/v3/core/clusternode"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/daemon/api"
	"github.com/opensvc/om3/v3/daemon/msgbus"
	"github.com/opensvc/om3/v3/util/pubsub"
)

func (a *DaemonAPI) PostInstanceActionSyncRestore(ctx echo.Context, nodename, namespace string, kind naming.Kind, name string, params api.PostInstanceActionSyncRestoreParams) error {
	if v, err := assertOperator(ctx, namespace); !v {
		return err
	}
	nodename = a.parseNodename(nodename)
	if nodename == a.localhost || nodename == "localhost" {
		return a.postLocalInstanceActionSyncRestore(ctx, namespace, kind, name, params)
	} else if !clusternode.Has(nodename) {
		return JSONProblemf(ctx, http.StatusBadRequest, "Invalid nodename", "field 'nodename' with value '%s' is not a cluster node", nodename)
	}
	return a.proxy(ctx, nodename, func(t *client.T) (*http.Response, error) {
		return t.PostInstanceActionSyncRestore(ctx.Request().Context(), nodename, namespace, kind, name, &params)
	})
}

func (a *DaemonAPI) postLocalInstanceActionSyncRestore(ctx echo.Context, namespace string, kind naming.Kind, name string, params api.PostInstanceActionSyncRestoreParams) error {
	log := LogHandler(ctx, "PostInstanceActionSyncRestore")
	var (
		requesterSid uuid.UUID
	)
	p, err := naming.NewPath(namespace, kind, name)
	if err != nil {
		return JSONProblemf(ctx, http.StatusBadRequest, "Invalid parameters", "%s", err)
	}
	log = naming.LogWithPath(log, p)
	args := []string{p.String(), "instance", "sync", "restore"}
	if params.Rid != nil && *params.Rid != "" {
		args = append(args, "--rid", *params.Rid)
	}
	if params.Subset != nil && *params.Subset != "" {
		args = append(args, "--subset", *params.Subset)
	}
	if params.Tag != nil && *params.Tag != "" {
		args = append(args, "--tag", *params.Tag)
	}
	if params.Src != nil && *params.Src != "" {
		args = append(args, "--src", *params.Src)
	}
	if params.SessionID != nil {
		requesterSid = *params.SessionID
	}
	tmpDir, err := os.MkdirTemp("", "restore-*")
	if err != nil {
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}
	defer os.RemoveAll(tmpDir)

	sub := a.Bus.Sub(fmt.Sprintf("api.post_instance_action_sync_restore %s", ctx.Get("uuid")))
	sub.AddFilter(&msgbus.ExecSuccess{}, pubsub.Label{"path", p.String()})
	sub.AddFilter(&msgbus.ExecFailed{}, pubsub.Label{"path", p.String()})
	sub.Start()
	defer func() {
		if err := sub.Stop(); err != nil {
			log.Errorf("sub stop %s", err)
		}
	}()

	args = append(args, "--to", tmpDir)
	_, execID, err := a.apiExec(ctx, p, requesterSid, args, log)
	if err != nil {
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}

	done := false
	for !done {
		select {
		case <-ctx.Request().Context().Done():
			return JSONProblemf(ctx, http.StatusInternalServerError, "Instance action sync restore", "%s", ctx.Request().Context().Err())
		case i := <-sub.C:
			switch msg := i.(type) {
			case *msgbus.ExecSuccess:
				if msg.ExecID.UUID() != execID {
					continue
				}
				done = true
				break
			case *msgbus.ExecFailed:
				if msg.ExecID.UUID() != execID {
					continue
				}
				log.Errorf("instance sync restore failed: %s", msg.ErrS)
				return JSONProblemf(ctx, http.StatusInternalServerError, "Instance action sync restore", "%s", msg.ErrS)
			}
		}
	}

	d, err := os.Open(tmpDir)
	if err != nil {
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}
	defer d.Close()

	_, err = d.ReadDir(1)
	if err != nil {
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}

	dst := tmpDir + ".tar.gz"
	if err = createTar(tmpDir, dst); err != nil {
		log.Errorf("instance sync restore failed: %s", err)
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}

	file, err := os.Open(dst)
	if err != nil {
		return JSONProblemf(ctx, http.StatusInternalServerError, "", "%s", err)
	}
	defer file.Close()

	return ctx.Stream(http.StatusOK, "application/octet-stream", file)
}

func createTar(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gzw := gzip.NewWriter(out)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}
