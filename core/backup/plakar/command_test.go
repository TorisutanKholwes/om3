package plakar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteConfig(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)

	b, err := os.ReadFile(filepath.Join(s.configDir(), "stores.yml"))
	require.NoError(t, err)
	require.Contains(t, string(b), "version: v1.0.0")
	require.Contains(t, string(b), "lab:")
	require.Contains(t, string(b), "location: "+s.location)
	require.NotContains(t, string(b), "passphrase")

	for _, name := range []string{"stores.yml", "sources.yml", "destinations.yml"} {
		info, err := os.Stat(filepath.Join(s.configDir(), name))
		require.NoError(t, err)
		require.Equal(t, filePerm, info.Mode().Perm(), name)
	}
	for _, dir := range []string{s.configDir(), s.cacheDir(), s.dataDir()} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		require.Equal(t, dirPerm, info.Mode().Perm(), dir)
	}
	for _, name := range []string{".first-run", ".disabled-security-check"} {
		_, err := os.Stat(filepath.Join(s.cacheDir(), name))
		require.NoError(t, err, name)
	}
}

func TestRunGlobalArgs(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}, quiet: true}))
	calls := f.rawCalls()
	require.Len(t, calls, 1)
	require.Equal(t, strings.Join([]string{
		"-configdir", s.configDir(),
		"-cachedir", s.cacheDir(),
		"-datadir", s.dataDir(),
		"-keyfile", s.keyFile(),
		"-quiet", "at", "@lab", "ls",
	}, " "), calls[0])
}

func TestRunErrors(t *testing.T) {
	t.Run("wrong passphrase", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		f.answer("ls", "plakar: could not open repository: invalid passphrase\n", 77)
		err := s.run(context.Background(), call{args: []string{"ls"}})
		require.ErrorIs(t, err, ErrAuth)
		require.Contains(t, err.Error(), "invalid passphrase")
	})
	t.Run("integrity", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		f.answer("check", "check failed for 1 snapshot(s)\n", 65)
		require.ErrorIs(t, s.run(context.Background(), call{args: []string{"check"}}), ErrIntegrity)
	})
	t.Run("other", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		f.answer("ls", "boom\n", 1)
		err := s.run(context.Background(), call{args: []string{"ls"}})
		var errCmd *ErrCommand
		require.True(t, errors.As(err, &errCmd))
		require.Equal(t, 1, errCmd.ExitCode)
		require.Equal(t, "boom", errCmd.Output)
	})
	t.Run("config error", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		s.configErr = ErrNoBinary
		require.ErrorIs(t, s.run(context.Background(), call{args: []string{"ls"}}), ErrNoBinary)
		require.Empty(t, f.calls())
	})
}

func TestRunRetriesUnresolved(t *testing.T) {
	f := newFakePlakar(t)
	s := newTestStore(t, f)
	require.NoError(t, os.WriteFile(filepath.Join(s.configDir(), "stores.yml"), nil, 0600))
	f.answerOnce("ls", "plakar: could not resolve repository: @lab\n", 1)
	require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}}))
	require.Equal(t, []string{"ls", "ls"}, f.calls())
	b, err := os.ReadFile(filepath.Join(s.configDir(), "stores.yml"))
	require.NoError(t, err)
	require.Contains(t, string(b), "lab:")
}

func TestPrepare(t *testing.T) {
	t.Run("create a store that can not be opened", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		s.prepared = false
		f.answerOnce("ls", "plakar: open: no such file or directory\n", 66)
		require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}}))
		require.Equal(t, []string{"ls -latest", "create", "ls"}, f.calls())
		require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}}))
		require.Equal(t, []string{"ls -latest", "create", "ls", "ls"}, f.calls())
	})
	t.Run("an existing store is not created", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		s.prepared = false
		require.NoError(t, s.run(context.Background(), call{args: []string{"ls"}}))
		require.Equal(t, []string{"ls -latest", "ls"}, f.calls())
	})
	t.Run("a wrong passphrase does not create", func(t *testing.T) {
		f := newFakePlakar(t)
		s := newTestStore(t, f)
		s.prepared = false
		f.answer("ls", "", 77)
		require.ErrorIs(t, s.run(context.Background(), call{args: []string{"ls"}}), ErrAuth)
		require.Equal(t, []string{"ls -latest"}, f.calls())
	})
}
