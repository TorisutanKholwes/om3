package object

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/core/keywords"
	"github.com/opensvc/om3/v3/core/naming"
	"github.com/opensvc/om3/v3/util/key"
)

// The node and cluster stores are not the core objects store, so a keyword
// declared there has to be declared here too. These are the ones the agent
// writes itself, or accepts in any section.
func TestNodeAndClusterKeywordLookup(t *testing.T) {
	cases := map[string]struct {
		store keywords.Store
		kind  naming.Kind
		key   key.T
	}{
		"the uuid the agent writes in node.conf": {
			store: NodeKeywordStore,
			kind:  naming.KindInvalid,
			key:   key.Parse("id"),
		},
		"the uuid the agent writes in cluster.conf": {
			store: ccfgKeywordStore,
			kind:  naming.KindCcfg,
			key:   key.Parse("id"),
		},
		"a node.conf DEFAULT comment": {
			store: NodeKeywordStore,
			kind:  naming.KindInvalid,
			key:   key.Parse("comment"),
		},
		"a node.conf section comment": {
			store: NodeKeywordStore,
			kind:  naming.KindInvalid,
			key:   key.Parse("node.comment"),
		},
		"a cluster.conf DEFAULT comment": {
			store: ccfgKeywordStore,
			kind:  naming.KindCcfg,
			key:   key.Parse("comment"),
		},
		"a cluster.conf driver section comment": {
			store: ccfgKeywordStore,
			kind:  naming.KindCcfg,
			key:   key.Parse("hb#1.comment"),
		},
		"the name listener.openid_issuer had before the rename": {
			store: ccfgKeywordStore,
			kind:  naming.KindCcfg,
			key:   key.Parse("listener.openid_authority"),
		},
	}
	for title, c := range cases {
		t.Run(title, func(t *testing.T) {
			require.NotNil(t, keywordLookup(c.store, c.key, c.kind, ""))
		})
	}
}

// A store is declared in a [store#<name>] section of cluster.conf, and its
// driver keywords are only known for the type the section declares.
func TestStoreKeywordLookup(t *testing.T) {
	for _, option := range []string{"type", "store", "passphrase", "identity", "host_key", "keep"} {
		t.Run(option, func(t *testing.T) {
			k := key.New("store#lab", option)
			require.NotNil(t, keywordLookup(ccfgKeywordStore, k, naming.KindCcfg, "plakar"))
		})
	}
	t.Run("plakar keywords need the plakar type", func(t *testing.T) {
		require.Nil(t, keywordLookup(ccfgKeywordStore, key.New("store#lab", "passphrase"), naming.KindCcfg, "other"))
	})
}
