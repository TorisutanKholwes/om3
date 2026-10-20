package plakar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/opensvc/om3/v3/util/command"
	"github.com/opensvc/om3/v3/util/funcopt"
)

type (
	// call is one plakar command, the global flags and the "at @<name>"
	// store selection excepted.
	call struct {
		args []string

		// stdin, if set, is the standard input of the command.
		stdin io.Reader

		// stdout, if set, receives the raw standard output of the
		// command, which is then not read as lines.
		stdout io.Writer

		// onLine, if set, receives each standard output line.
		onLine func(string)

		// quiet sets the plakar -quiet global flag, hiding the per file
		// lines of a backup or a restore.
		quiet bool
	}

	// outputTail keeps the last lines a command printed, to say why it
	// failed.
	outputTail struct {
		sync.Mutex
		lines []string
	}

	// ErrCommand is the error of a plakar command exiting with a non zero
	// status.
	ErrCommand struct {
		Args     []string
		ExitCode int
		Output   string
		Err      error
	}
)

const (
	// exit codes of plakar, from its exitcodes package
	exitIntegrityFailure = 65
	exitCannotOpen       = 66
	exitAuth             = 77

	outputTailSize = 20
)

var (
	// ErrAuth is returned when plakar refuses the passphrase, or refuses
	// to open a plaintext store.
	ErrAuth = errors.New("the store refused the passphrase")

	// ErrIntegrity is returned when a check finds a corrupted snapshot.
	ErrIntegrity = errors.New("the store integrity check failed")

	// ErrCannotOpen is returned when plakar can not open the store, which
	// is the case of a store not created yet.
	ErrCannotOpen = errors.New("the store can not be opened")
)

func (t *outputTail) add(s string) {
	t.Lock()
	defer t.Unlock()
	t.lines = append(t.lines, s)
	if len(t.lines) > outputTailSize {
		t.lines = t.lines[len(t.lines)-outputTailSize:]
	}
}

func (t *outputTail) String() string {
	t.Lock()
	defer t.Unlock()
	return strings.Join(t.lines, "\n")
}

func (t *ErrCommand) Error() string {
	s := fmt.Sprintf("plakar %s: exit code %d", strings.Join(t.Args, " "), t.ExitCode)
	if t.Err != nil {
		s += ": " + t.Err.Error()
	}
	if t.Output != "" {
		s += ": " + t.Output
	}
	return s
}

func (t *ErrCommand) Unwrap() error {
	return t.Err
}

// isUnresolved returns true if plakar could not find the store in
// stores.yml.
func (t *ErrCommand) isUnresolved() bool {
	return strings.Contains(t.Output, "could not resolve repository")
}

// globalArgs returns the plakar flags preceding the subcommand.
func (t *T) globalArgs(quiet bool) []string {
	l := []string{
		"-configdir", t.configDir(),
		"-cachedir", t.cacheDir(),
		"-datadir", t.dataDir(),
		"-keyfile", t.keyFile(),
	}
	if quiet {
		l = append(l, "-quiet")
	}
	return append(l, "at", "@"+t.name)
}

// run runs a plakar command on the store, creating the store first if it
// does not exist yet.
func (t *T) run(ctx context.Context, c call) error {
	if err := t.prepare(ctx); err != nil {
		return err
	}
	return t.runPrepared(ctx, c)
}

// runPrepared runs a plakar command on the store.
//
// If plakar can not resolve the store, the configuration files are
// written again and the command is retried once. A command reading a
// stream is not retried, its input being consumed.
func (t *T) runPrepared(ctx context.Context, c call) error {
	if t.configErr != nil {
		return t.configErr
	}
	if err := t.installKey(); err != nil {
		return err
	}
	err := t.exec(ctx, c)
	var errCmd *ErrCommand
	if c.stdin == nil && errors.As(err, &errCmd) && errCmd.isUnresolved() {
		t.log.Infof("plakar could not resolve the store, write its configuration again and retry")
		if err := t.writeConfig(); err != nil {
			return err
		}
		err = t.exec(ctx, c)
	}
	return err
}

// exec runs a plakar command once.
//
// The command line holds the path of the key file, never the passphrase.
// plakar prints the errors of a running command on its standard output,
// and its setup errors on its standard error: the last lines of both are
// kept to say why a command failed.
func (t *T) exec(ctx context.Context, c call) error {
	var tail outputTail
	env, cleanup, err := t.env(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	args := append(t.globalArgs(c.quiet), c.args...)
	opts := []funcopt.O{
		command.WithName(t.bin),
		command.WithContext(ctx),
		command.WithArgs(args),
		command.WithEnv(env),
		command.WithLogger(t.log),
		command.WithCommandLogLevel(zerolog.InfoLevel),
		command.WithStderrLogLevel(zerolog.WarnLevel),
		command.WithOnStderrLine(tail.add),
	}
	if c.stdout == nil {
		onLine := tail.add
		if c.onLine != nil {
			onLine = func(s string) {
				tail.add(s)
				c.onLine(s)
			}
		} else {
			opts = append(opts, command.WithStdoutLogLevel(zerolog.InfoLevel))
		}
		opts = append(opts, command.WithOnStdoutLine(onLine))
	}
	cmd := command.New(opts...)
	if c.stdin != nil {
		cmd.Cmd().Stdin = c.stdin
	}
	if c.stdout != nil {
		cmd.Cmd().Stdout = c.stdout
	}
	if err := cmd.Run(); err != nil {
		errCmd := &ErrCommand{
			Args:     c.args,
			ExitCode: cmd.NormalizedExitCode(),
			Output:   tail.String(),
		}
		switch errCmd.ExitCode {
		case exitAuth:
			errCmd.Err = ErrAuth
		case exitIntegrityFailure:
			errCmd.Err = ErrIntegrity
		case exitCannotOpen:
			errCmd.Err = ErrCannotOpen
		default:
			if ctx.Err() != nil {
				errCmd.Err = ctx.Err()
			}
		}
		return errCmd
	}
	return nil
}

// env returns the environment variables added to the plakar commands, and
// the function releasing what was set up for them.
//
// PLAKAR_PASSPHRASE is emptied: -keyfile has precedence over it, but an
// environment variable is readable in /proc/<pid>/environ and inherited by
// the subprocesses, so the agent one must not reach plakar. PLAKAR_TAGS is
// emptied too, as a backup without -tag would use it.
func (t *T) env(ctx context.Context) ([]string, func(), error) {
	env := []string{
		"PLAKAR_PASSPHRASE=",
		"PLAKAR_TAGS=",
	}
	return env, func() {}, nil
}

// prepare makes sure the store exists, creating it if plakar can not open
// it, once per driver allocation.
func (t *T) prepare(ctx context.Context) error {
	if t.configErr != nil {
		return t.configErr
	}
	t.preparedMu.Lock()
	defer t.preparedMu.Unlock()
	if t.prepared {
		return nil
	}
	err := t.runPrepared(ctx, call{args: []string{"ls", "-latest"}, onLine: func(string) {}})
	switch {
	case err == nil:
	case errors.Is(err, ErrCannotOpen):
		t.log.Infof("create the store at %s", t.location)
		if err := t.runPrepared(ctx, call{args: []string{"create"}}); err != nil {
			return fmt.Errorf("store %s: create: %w", t.name, err)
		}
	default:
		return fmt.Errorf("store %s: %w", t.name, err)
	}
	t.prepared = true
	return nil
}
