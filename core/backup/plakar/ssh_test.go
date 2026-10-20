package plakar

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostKeyInStoresConfig(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	s.hostKey = "from backup/sec/lab key host_key"
	s.keyDecoder = func(option, ref string) ([]byte, error) {
		require.Equal(t, "host_key", option)
		return []byte("store.lab ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"), nil
	}
	require.NoError(t, s.writeConfig())
	b, err := os.ReadFile(filepath.Join(s.configDir(), "stores.yml"))
	require.NoError(t, err)
	require.Contains(t, string(b), "host_key: store.lab ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl")

	s.keyDecoder = func(option, ref string) ([]byte, error) {
		return []byte("a\nb\n"), nil
	}
	require.Error(t, s.writeConfig())
}

func TestIdentityAgent(t *testing.T) {
	for _, bin := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id")
	require.NoError(t, exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyFile).Run())
	key, err := os.ReadFile(keyFile)
	require.NoError(t, err)

	f := newFakePlakar(t)
	// the fake plakar lists the agent keys in place of an ls
	f.write("ls.capture", "")
	s := newTestStore(t, f)
	s.identity = "from backup/sec/lab key identity"
	s.keyDecoder = func(option, ref string) ([]byte, error) {
		require.Equal(t, "identity", option)
		return key, nil
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_PLAKAR_DIR/calls\"\nssh-add -l > \"$FAKE_PLAKAR_DIR/agent\"\n"
	require.NoError(t, os.WriteFile(f.bin(), []byte(script), 0700))

	require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}}))
	b, err := os.ReadFile(filepath.Join(f.dir, "agent"))
	require.NoError(t, err)
	require.Contains(t, string(b), "ED25519")

	// the agent is gone with the command
	entries, err := os.ReadDir(filepath.Join(s.runDir, "ssh"))
	require.NoError(t, err)
	require.Empty(t, entries)
}
