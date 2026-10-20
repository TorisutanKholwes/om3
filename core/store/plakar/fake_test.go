package plakar

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/opensvc/om3/v3/util/plog"
)

type (
	// fakePlakar is a plakar stand-in script recording its calls, and
	// answering each subcommand with a canned output and exit code.
	fakePlakar struct {
		t   *testing.T
		dir string
	}
)

// fakeScript is the plakar stand-in. The subcommand is the word following
// the "@<name>" store selection.
//
// For a subcommand <sub>, it prints <sub>.out, exits with the code in
// <sub>.exit, or once with the code in <sub>.once.exit, and saves its
// standard input in <sub>.stdin when <sub>.capture exists.
const fakeScript = `#!/bin/sh
d="$FAKE_PLAKAR_DIR"
printf '%s\n' "$*" >> "$d/calls"
found=0
sub=""
for a in "$@"; do
	if [ $found = 1 ]; then sub="$a"; break; fi
	case "$a" in @*) found=1;; esac
done
if [ -f "$d/$sub.capture" ]; then cat > "$d/$sub.stdin"; fi
if [ -f "$d/$sub.once.out" ]; then cat "$d/$sub.once.out"; rm -f "$d/$sub.once.out"
elif [ -f "$d/$sub.out" ]; then cat "$d/$sub.out"; fi
if [ -f "$d/$sub.once.exit" ]; then code=$(cat "$d/$sub.once.exit"); rm -f "$d/$sub.once.exit"; exit $code; fi
if [ -f "$d/$sub.exit" ]; then exit $(cat "$d/$sub.exit"); fi
exit 0
`

func newFakePlakar(t *testing.T) *fakePlakar {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plakar"), []byte(fakeScript), 0700))
	t.Setenv("FAKE_PLAKAR_DIR", dir)
	return &fakePlakar{t: t, dir: dir}
}

func (f *fakePlakar) bin() string {
	return filepath.Join(f.dir, "plakar")
}

func (f *fakePlakar) write(name, content string) {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(filepath.Join(f.dir, name), []byte(content), 0600))
}

// answer sets the output and the exit code of a subcommand.
func (f *fakePlakar) answer(sub, out string, code int) {
	f.write(sub+".out", out)
	f.write(sub+".exit", strconv.Itoa(code))
}

// answerOnce sets the output and the exit code of the next call of a
// subcommand.
func (f *fakePlakar) answerOnce(sub, out string, code int) {
	f.write(sub+".once.out", out)
	f.write(sub+".once.exit", strconv.Itoa(code))
}

// capture makes the subcommand save its standard input.
func (f *fakePlakar) capture(sub string) {
	f.write(sub+".capture", "")
}

func (f *fakePlakar) stdin(sub string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, sub+".stdin"))
	require.NoError(f.t, err)
	return string(b)
}

// calls returns the argument lists the script was called with, without the
// global flags and the store selection.
func (f *fakePlakar) calls() []string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(f.t, err)
	l := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if _, after, ok := strings.Cut(line, " at @lab "); ok {
			line = after
		}
		l = append(l, line)
	}
	return l
}

// rawCalls returns the full argument lists the script was called with.
func (f *fakePlakar) rawCalls() []string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, "calls"))
	require.NoError(f.t, err)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// newTestStore returns the "lab" store, configured in temporary
// directories, running the fake plakar.
func newTestStore(t *testing.T, f *fakePlakar) *T {
	t.Helper()
	dir := t.TempDir()
	s := &T{
		name:       "lab",
		location:   filepath.Join(dir, "kloset"),
		passphrase: "from store/sec/lab key passphrase",
		root:       filepath.Join(dir, "var"),
		runDir:     filepath.Join(dir, "run"),
		bin:        f.bin(),
		log:        plog.NewDefaultLogger(),
		installKey: func() error { return nil },
	}
	require.NoError(t, s.writeConfig())
	// the store exists unless a test says otherwise
	s.prepared = true
	return s
}
