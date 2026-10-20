package ressyncbackup

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/backup"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/core/object"
	"github.com/opensvc/om3/v3/core/xconfig"
)

func TestReferencedPaths(t *testing.T) {
	cfg, err := xconfig.NewObject("", []byte(`
[app#1]
secrets_environment = CRT=cert1/server.pem "sec1/*"
configs_environment = PORT=http/port {name}/*
[volume#1]
configs = conf/mycnf:/etc/mysql/my.cnf:ro
secrets@node1 = tls/key:/etc/key
install = /etc/ from other/cfg/haproxy key haproxy.cfg
[sync#1]
passphrase = from ./sec/lab key passphrase
`))
	require.NoError(t, err)
	l := referencedPaths(cfg, "test")
	s := make([]string, 0, len(l))
	for _, p := range l {
		s = append(s, p.String())
	}
	require.Equal(t, []string{
		"other/cfg/haproxy",
		"test/cfg/conf",
		"test/cfg/http",
		"test/sec/cert1",
		"test/sec/lab",
		"test/sec/sec1",
		"test/sec/tls",
	}, s)
}

func TestUpdateIncludeRefs(t *testing.T) {
	r, store, ctx, _ := setup(t, "include_refs = true\nkeep = 1d\n\n[app#1]\nconfigs_environment = A=s1/a\n")
	sec, err := object.NewCfg(naming.Path{Namespace: "test", Kind: naming.KindCfg, Name: "s1"})
	require.NoError(t, err)
	require.NoError(t, sec.AddKey("a", []byte("va")))
	require.NoError(t, sec.AddKey("b/c", []byte("vbc")))

	require.NoError(t, r.Update(ctx))
	require.Equal(t, "va", store.streams["refs/test/cfg/s1/a"])
	require.Equal(t, "vbc", store.streams["refs/test/cfg/s1/b/c"])
	require.Contains(t, store.prunes, backup.Tags{"path": "test/svc/app1", "rid": "ref", "ref": "test/cfg/s1:a"})
	require.Contains(t, store.prunes, backup.Tags{"path": "test/svc/app1", "rid": "ref", "ref": "test/cfg/s1:b/c"})
}

func TestUpdateIncludeRefsMissingObject(t *testing.T) {
	r, _, ctx, _ := setup(t, "include_refs = true\n\n[app#1]\nconfigs_environment = A=nope/a\n")
	require.ErrorContains(t, r.Update(ctx), "refs")
}
