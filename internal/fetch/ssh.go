package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	DefaultSSHPort           = 22
	DefaultSSHConnectTimeout = 10 * time.Second
	DefaultSSHCommandTimeout = 15 * time.Second
	DefaultSSHMaxOutputBytes = int64(1 << 20)
	DefaultSSHMaxStderrBytes = int64(64 << 10)
)

// SSHCommandConfig contains the connection settings for one SSH command.
// Authentication is intentionally limited to one private key.
type SSHCommandConfig struct {
	Host           string
	Port           int
	Username       string
	PrivateKeyFile string
	KnownHostsFile string
	ConnectTimeout time.Duration
	CommandTimeout time.Duration
	MaxOutputBytes int64
}

// RunSSHCommand opens a new SSH connection, runs one command without a PTY,
// and closes the connection before returning.
func RunSSHCommand(
	ctx context.Context,
	cfg SSHCommandConfig,
	command string,
) (
	stdout []byte,
	stderr []byte,
	err error,
) {
	cfg, err = normalizeSSHCommandConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("SSH command canceled before connect: %w", err)
	}
	if command == "" {
		return nil, nil, errors.New("SSH command must not be empty")
	}

	signer, err := loadSSHSigner(cfg.PrivateKeyFile)
	if err != nil {
		return nil, nil, err
	}

	hostKeyCallback, err := knownhosts.New(cfg.KnownHostsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load known_hosts: %w", err)
	}

	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	sshConfig := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.ConnectTimeout,
	}

	client, err := dialSSHClient(ctx, address, cfg.ConnectTimeout, sshConfig)
	if err != nil {
		return nil, nil, err
	}

	var closeOnce sync.Once
	closeClient := func() {
		closeOnce.Do(func() {
			_ = client.Close()
		})
	}
	defer closeClient()

	var commandTimedOut atomic.Bool
	timeoutCallbackDone := make(chan struct{})
	timeoutTimer := time.AfterFunc(cfg.CommandTimeout, func() {
		defer close(timeoutCallbackDone)
		commandTimedOut.Store(true)
		closeClient()
	})

	var contextCanceled atomic.Bool
	contextCallbackDone := make(chan struct{})
	stopContextCancel := context.AfterFunc(ctx, func() {
		defer close(contextCallbackDone)
		contextCanceled.Store(true)
		closeClient()
	})
	var stopCommandWatchersOnce sync.Once
	stopCommandWatchers := func() {
		stopCommandWatchersOnce.Do(func() {
			if !timeoutTimer.Stop() {
				<-timeoutCallbackDone
			}
			if !stopContextCancel() {
				<-contextCallbackDone
			}
		})
	}
	defer stopCommandWatchers()

	session, err := client.NewSession()
	if err != nil {
		stopCommandWatchers()
		if contextCanceled.Load() {
			return nil, nil, fmt.Errorf("SSH command canceled: %w", ctx.Err())
		}
		if commandTimedOut.Load() {
			return nil, nil, fmt.Errorf("SSH command timed out after %s", cfg.CommandTimeout)
		}
		return nil, nil, fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	stdoutBuffer := newBoundedBuffer(cfg.MaxOutputBytes, closeClient)
	stderrBuffer := newBoundedBuffer(DefaultSSHMaxStderrBytes, closeClient)
	session.Stdout = stdoutBuffer
	session.Stderr = stderrBuffer

	runErr := session.Run(command)
	stopCommandWatchers()

	stdout = stdoutBuffer.Bytes()
	stderr = stderrBuffer.Bytes()

	if stdoutBuffer.Exceeded() {
		return stdout, stderr, fmt.Errorf("remote stdout exceeded %d bytes", cfg.MaxOutputBytes)
	}
	if stderrBuffer.Exceeded() {
		return stdout, stderr, fmt.Errorf("remote stderr exceeded %d bytes", DefaultSSHMaxStderrBytes)
	}
	if contextCanceled.Load() {
		return stdout, stderr, fmt.Errorf("SSH command canceled: %w", ctx.Err())
	}
	if commandTimedOut.Load() {
		return stdout, stderr, fmt.Errorf("SSH command timed out after %s", cfg.CommandTimeout)
	}
	if runErr != nil {
		return stdout, stderr, formatSSHCommandError(runErr, stderr)
	}

	return stdout, stderr, nil
}

func normalizeSSHCommandConfig(cfg SSHCommandConfig) (SSHCommandConfig, error) {
	if cfg.Host == "" {
		return cfg, errors.New("SSH host must not be empty")
	}
	if cfg.Username == "" {
		return cfg, errors.New("SSH username must not be empty")
	}
	if cfg.PrivateKeyFile == "" {
		return cfg, errors.New("SSH private key file must not be empty")
	}
	if cfg.KnownHostsFile == "" {
		return cfg, errors.New("SSH known_hosts file must not be empty")
	}

	if cfg.Port == 0 {
		cfg.Port = DefaultSSHPort
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("SSH port must be between 1 and 65535: %d", cfg.Port)
	}

	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = DefaultSSHConnectTimeout
	}
	if cfg.ConnectTimeout < 0 {
		return cfg, errors.New("SSH connect timeout must be positive")
	}

	if cfg.CommandTimeout == 0 {
		cfg.CommandTimeout = DefaultSSHCommandTimeout
	}
	if cfg.CommandTimeout < 0 {
		return cfg, errors.New("SSH command timeout must be positive")
	}

	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = DefaultSSHMaxOutputBytes
	}
	if cfg.MaxOutputBytes < 0 || cfg.MaxOutputBytes > DefaultSSHMaxOutputBytes {
		return cfg, fmt.Errorf(
			"SSH max output bytes must be between 1 and %d: %d",
			DefaultSSHMaxOutputBytes,
			cfg.MaxOutputBytes,
		)
	}

	return cfg, nil
}

func loadSSHSigner(privateKeyFile string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read SSH private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		var passphraseMissingError *ssh.PassphraseMissingError
		if errors.As(err, &passphraseMissingError) {
			return nil, errors.New("encrypted private keys are not supported; use a dedicated restricted key")
		}
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}

	return signer, nil
}

func dialSSHClient(
	ctx context.Context,
	address string,
	connectTimeout time.Duration,
	sshConfig *ssh.ClientConfig,
) (*ssh.Client, error) {
	return dialSSHClientWithDialer(
		ctx,
		address,
		connectTimeout,
		sshConfig,
		(&net.Dialer{}).DialContext,
	)
}

func dialSSHClientWithDialer(
	ctx context.Context,
	address string,
	connectTimeout time.Duration,
	sshConfig *ssh.ClientConfig,
	dialContext func(context.Context, string, string) (net.Conn, error),
) (*ssh.Client, error) {
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	netConn, err := dialContext(connectCtx, "tcp", address)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SSH connect canceled: %w", ctx.Err())
		}
		if connectCtx.Err() != nil {
			return nil, fmt.Errorf("SSH connect timed out after %s: %w", connectTimeout, connectCtx.Err())
		}
		return nil, fmt.Errorf("SSH connect to %s: %w", address, err)
	}

	deadline, _ := connectCtx.Deadline()
	if err := netConn.SetDeadline(deadline); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("set SSH handshake deadline: %w", err)
	}

	stopConnectCancel := context.AfterFunc(connectCtx, func() {
		_ = netConn.Close()
	})
	clientConn, channels, requests, err := ssh.NewClientConn(netConn, address, sshConfig)
	cancelTriggered := !stopConnectCancel()
	if err != nil {
		_ = netConn.Close()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SSH handshake canceled: %w", ctx.Err())
		}
		if connectCtx.Err() != nil || cancelTriggered || isTimeoutError(err) {
			return nil, fmt.Errorf("SSH handshake timed out after %s", connectTimeout)
		}
		return nil, fmt.Errorf("SSH handshake with %s: %w", address, err)
	}
	if cancelTriggered || connectCtx.Err() != nil {
		_ = clientConn.Close()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SSH handshake canceled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("SSH handshake timed out after %s", connectTimeout)
	}

	if err := netConn.SetDeadline(time.Time{}); err != nil {
		_ = clientConn.Close()
		return nil, fmt.Errorf("clear SSH handshake deadline: %w", err)
	}

	return ssh.NewClient(clientConn, channels, requests), nil
}

func isTimeoutError(err error) bool {
	var netError net.Error
	return errors.As(err, &netError) && netError.Timeout()
}

func formatSSHCommandError(runErr error, stderr []byte) error {
	var exitError *ssh.ExitError
	if !errors.As(runErr, &exitError) {
		return fmt.Errorf("run SSH command: %w", runErr)
	}

	message := strings.TrimSpace(string(stderr))
	if message == "" {
		return fmt.Errorf("remote command failed with exit status %d", exitError.ExitStatus())
	}
	return fmt.Errorf("remote command failed with exit status %d: %s", exitError.ExitStatus(), message)
}

type boundedBuffer struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	limit      int64
	exceeded   bool
	onExceeded func()
	once       sync.Once
}

func newBoundedBuffer(limit int64, onExceeded func()) *boundedBuffer {
	return &boundedBuffer{
		limit:      limit,
		onExceeded: onExceeded,
	}
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	if buffer.exceeded {
		buffer.mu.Unlock()
		return 0, errors.New("output limit exceeded")
	}

	remaining := buffer.limit - int64(buffer.buffer.Len())
	if int64(len(data)) <= remaining {
		written, err := buffer.buffer.Write(data)
		buffer.mu.Unlock()
		return written, err
	}

	written := 0
	if remaining > 0 {
		written, _ = buffer.buffer.Write(data[:int(remaining)])
	}
	buffer.exceeded = true
	buffer.mu.Unlock()

	buffer.once.Do(func() {
		if buffer.onExceeded != nil {
			buffer.onExceeded()
		}
	})

	return written, errors.New("output limit exceeded")
}

func (buffer *boundedBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Clone(buffer.buffer.Bytes())
}

func (buffer *boundedBuffer) Exceeded() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.exceeded
}
