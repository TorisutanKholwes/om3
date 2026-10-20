package plakar

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	// agentSeq makes the agent socket names unique among the commands
	// of a process.
	agentSeq atomic.Uint64

	// agentStartTimeout is how long an agent has to create its socket.
	agentStartTimeout = 5 * time.Second
)

// startAgent starts a SSH agent holding the identity key for the duration of
// one plakar command, and returns the SSH_AUTH_SOCK value to pass plakar and
// the function stopping the agent.
//
// The sftp package of plakar runs the ssh binary of the node, which finds the
// key in the agent: the key is never written to disk, where the
// ssh_private_key option of stores.yml would have put it.
func (t *T) startAgent(ctx context.Context) (string, func(), error) {
	key, err := t.decodeKey("identity", t.identity)
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Join(t.runDir, "ssh")
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", nil, err
	}
	sock := filepath.Join(dir, "agent."+strconv.Itoa(os.Getpid())+"."+strconv.FormatUint(agentSeq.Add(1), 10))
	_ = os.Remove(sock)

	agent := exec.Command("ssh-agent", "-D", "-a", sock)
	if err := agent.Start(); err != nil {
		return "", nil, fmt.Errorf("store %s: start ssh-agent: %w", t.name, err)
	}
	stop := func() {
		_ = agent.Process.Kill()
		_ = agent.Wait()
		_ = os.Remove(sock)
	}
	deadline := time.Now().Add(agentStartTimeout)
	for {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			stop()
			return "", nil, fmt.Errorf("store %s: ssh-agent did not create its socket", t.name)
		}
		time.Sleep(10 * time.Millisecond)
	}

	add := exec.CommandContext(ctx, "ssh-add", "-q", "-")
	add.Env = append(os.Environ(), "SSH_AUTH_SOCK="+sock)
	add.Stdin = bytes.NewReader(append(bytes.TrimRight(key, "\n"), '\n'))
	if out, err := add.CombinedOutput(); err != nil {
		stop()
		return "", nil, fmt.Errorf("store %s: load the identity in ssh-agent: %w: %s", t.name, err, strings.TrimSpace(string(out)))
	}
	return sock, stop, nil
}

// hostKeyOption returns the known_hosts line of the host_key keyword, the
// value of the sftp package host_key option.
func (t *T) hostKeyOption() (string, error) {
	b, err := t.decodeKey("host_key", t.hostKey)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" || strings.Contains(s, "\n") {
		return "", fmt.Errorf("store %s: host_key: must be a single known_hosts line", t.name)
	}
	return s, nil
}
