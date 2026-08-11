package oxcmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/opensvc/om3/v3/core/client"
	"github.com/opensvc/om3/v3/core/commoncmd"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/objectaction"
	"github.com/opensvc/om3/v3/daemon/api"
	"github.com/opensvc/om3/v3/util/xsession"
)

type (
	CmdObjectInstanceSyncRestore struct {
		OptsGlobal
		commoncmd.OptsAsync
		commoncmd.OptsLock
		commoncmd.OptsResourceSelector
		NodeSelector string
		Force        bool
		To           string
		Src          string
	}
)

func (t *CmdObjectInstanceSyncRestore) Run(kind string) error {
	mergedSelector := commoncmd.MergeSelector("", t.ObjectSelector, kind, "")
	return objectaction.New(
		objectaction.WithObjectSelector(mergedSelector),
		objectaction.WithRID(t.RID),
		objectaction.WithTag(t.Tag),
		objectaction.WithSubset(t.Subset),
		objectaction.WithOutput(t.Output),
		objectaction.WithColor(t.Color),
		objectaction.WithIgnoreNotFound(t.IgnoreNotFound),
		objectaction.WithAsyncTime(t.Time),
		objectaction.WithAsyncWait(t.Wait),
		objectaction.WithAsyncWatch(t.Watch),
		objectaction.WithRemoteNodes(t.NodeSelector),
		objectaction.WithRemoteFunc(func(ctx context.Context, p naming.Path, nodename string) (any, error) {
			c, err := client.New()
			if err != nil {
				return nil, err
			}
			params := api.PostInstanceActionSyncRestoreParams{}
			if t.OptsResourceSelector.RID != "" {
				params.Rid = &t.OptsResourceSelector.RID
			}
			if t.OptsResourceSelector.Subset != "" {
				params.Subset = &t.OptsResourceSelector.Subset
			}
			if t.OptsResourceSelector.Tag != "" {
				params.Tag = &t.OptsResourceSelector.Tag
			}
			if t.Src != "" {
				params.Src = &t.Src
			}
			{
				sid := xsession.Sid().UUID()
				params.SessionId = &sid
			}
			if t.To == "" {
				return nil, errors.New("missing --to parameter")
			}
			response, err := c.PostInstanceActionSyncRestoreWithResponse(ctx, nodename, p.Namespace, p.Kind, p.Name, &params)
			if err != nil {
				return nil, err
			}
			switch {
			case response.Body != nil && response.StatusCode() == http.StatusOK:
				outputFile := fmt.Sprintf("%s_%s_%s_%s.tar.gz", p.Namespace, p.Kind, p.Name, time.Now().Format("2006-01-02_15-04-05"))
				if err := os.WriteFile(filepath.Join(t.To, outputFile), response.Body, 0644); err != nil {
					return nil, fmt.Errorf("%s: node %s: write file %s: %w", p, nodename, outputFile, err)
				}
				return nil, nil
			case response.JSON401 != nil:
				return nil, fmt.Errorf("%s: node %s: %s", p, nodename, *response.JSON401)
			case response.JSON403 != nil:
				return nil, fmt.Errorf("%s: node %s: %s", p, nodename, *response.JSON403)
			case response.JSON500 != nil:
				return nil, fmt.Errorf("%s: node %s: %s", p, nodename, *response.JSON500)
			default:
				return nil, fmt.Errorf("%s: node %s: unexpected response: %s", p, nodename, response.Status())
			}
		}),
	).Do()
}
