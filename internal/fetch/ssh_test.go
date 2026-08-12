package fetch

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestRunSSHCommandSuccess(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())

	stdout, stderr, err := RunSSHCommand(context.Background(), cfg, "success")
	if err != nil {
		t.Fatalf("RunSSHCommand() error = %v", err)
	}
	if got, want := string(stdout), `{"ok":true}`; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if len(stderr) != 0 {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	select {
	case command := <-server.commands:
		if command != "success" {
			t.Fatalf("remote command = %q, want success", command)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive the command")
	}
	if server.ptyRequests.Load() != 0 {
		t.Fatalf("PTY requests = %d, want 0", server.ptyRequests.Load())
	}
}

func TestRunSSHCommandUsesNewConnectionEachTime(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())

	for range 2 {
		if _, _, err := RunSSHCommand(context.Background(), cfg, "success"); err != nil {
			t.Fatalf("RunSSHCommand() error = %v", err)
		}
	}
	if got := server.acceptedConnections.Load(); got != 2 {
		t.Fatalf("accepted connections = %d, want 2", got)
	}
}

func TestRunSSHCommandRejectsUnknownAndMismatchedHostKeys(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())

	t.Run("unknown host", func(t *testing.T) {
		cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, nil)
		_, _, err := RunSSHCommand(context.Background(), cfg, "success")
		if err == nil || !strings.Contains(err.Error(), "knownhosts: key is unknown") {
			t.Fatalf("error = %v, want unknown host key error", err)
		}
	})

	t.Run("mismatched host key", func(t *testing.T) {
		_, wrongHostSigner := newTestSigner(t)
		cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, wrongHostSigner.PublicKey())
		_, _, err := RunSSHCommand(context.Background(), cfg, "success")
		if err == nil || !strings.Contains(err.Error(), "knownhosts: key mismatch") {
			t.Fatalf("error = %v, want mismatched host key error", err)
		}
	})
}

func TestRunSSHCommandRejectsFailedSSHAuthentication(t *testing.T) {
	clientPrivateKey, _ := newTestSigner(t)
	_, allowedClientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, allowedClientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())

	_, _, err := RunSSHCommand(context.Background(), cfg, "success")
	if err == nil || !strings.Contains(err.Error(), "SSH handshake") {
		t.Fatalf("error = %v, want SSH authentication handshake error", err)
	}
}

func TestRunSSHCommandRejectsInvalidPrivateKeys(t *testing.T) {
	validPrivateKey, _ := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	knownHostsFile := writeKnownHostsFile(t, "127.0.0.1:22", hostSigner.PublicKey())

	tests := []struct {
		name           string
		privateKeyFile func(t *testing.T) string
		wantError      string
	}{
		{
			name: "missing file",
			privateKeyFile: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing-key")
			},
			wantError: "read SSH private key",
		},
		{
			name: "invalid contents",
			privateKeyFile: func(t *testing.T) string {
				return writeTestFile(t, "invalid-key", []byte("not a private key"))
			},
			wantError: "parse SSH private key",
		},
		{
			name: "encrypted key",
			privateKeyFile: func(t *testing.T) string {
				return writePrivateKeyFile(t, validPrivateKey, []byte("test-passphrase"))
			},
			wantError: "encrypted private keys are not supported; use a dedicated restricted key",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := SSHCommandConfig{
				Host:           "127.0.0.1",
				Port:           22,
				Username:       "test-user",
				PrivateKeyFile: test.privateKeyFile(t),
				KnownHostsFile: knownHostsFile,
			}
			_, _, err := RunSSHCommand(context.Background(), cfg, "success")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, test.wantError)
			}
		})
	}
}

func TestRunSSHCommandRejectsMissingKnownHosts(t *testing.T) {
	clientPrivateKey, _ := newTestSigner(t)
	cfg := SSHCommandConfig{
		Host:           "127.0.0.1",
		Port:           22,
		Username:       "test-user",
		PrivateKeyFile: writePrivateKeyFile(t, clientPrivateKey, nil),
		KnownHostsFile: filepath.Join(t.TempDir(), "missing-known-hosts"),
	}

	_, _, err := RunSSHCommand(context.Background(), cfg, "success")
	if err == nil || !strings.Contains(err.Error(), "load known_hosts") {
		t.Fatalf("error = %v, want missing known_hosts error", err)
	}
}

func TestRunSSHCommandHandshakeTimeout(t *testing.T) {
	clientPrivateKey, _ := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startHangingTCPServer(t)
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())
	cfg.ConnectTimeout = 50 * time.Millisecond

	startedAt := time.Now()
	_, _, err := RunSSHCommand(context.Background(), cfg, "success")
	if err == nil || !strings.Contains(err.Error(), "SSH handshake timed out") {
		t.Fatalf("error = %v, want SSH handshake timeout", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("handshake timeout took %s, want under 1s", elapsed)
	}
}

func TestDialSSHClientTCPConnectTimeout(t *testing.T) {
	t.Parallel()

	dialContext := func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := dialSSHClientWithDialer(
		context.Background(),
		"127.0.0.1:22",
		20*time.Millisecond,
		&ssh.ClientConfig{},
		dialContext,
	)
	if err == nil || !strings.Contains(err.Error(), "SSH connect timed out") {
		t.Fatalf("error = %v, want TCP connect timeout", err)
	}
}

func TestRunSSHCommandCommandTimeout(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())
	cfg.CommandTimeout = 50 * time.Millisecond

	startedAt := time.Now()
	_, _, err := RunSSHCommand(context.Background(), cfg, "hang")
	if err == nil || !strings.Contains(err.Error(), "SSH command timed out") {
		t.Fatalf("error = %v, want command timeout", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("command timeout took %s, want under 1s", elapsed)
	}
}

func TestRunSSHCommandContextCancelClosesConnection(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())
	cfg.CommandTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		err error
	}
	resultChannel := make(chan result, 1)
	go func() {
		_, _, err := RunSSHCommand(ctx, cfg, "hang")
		resultChannel <- result{err: err}
	}()

	select {
	case <-server.commands:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("server did not receive the hanging command")
	}

	select {
	case result := <-resultChannel:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunSSHCommand did not return after context cancellation")
	}
}

func TestRunSSHCommandEnforcesOutputLimits(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())

	tests := []struct {
		name      string
		command   string
		wantError string
		wantBytes int
	}{
		{
			name:      "stdout",
			command:   "large-stdout",
			wantError: "remote stdout exceeded 1048576 bytes",
			wantBytes: int(DefaultSSHMaxOutputBytes),
		},
		{
			name:      "stderr",
			command:   "large-stderr",
			wantError: "remote stderr exceeded 65536 bytes",
			wantBytes: int(DefaultSSHMaxStderrBytes),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := RunSSHCommand(context.Background(), cfg, test.command)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, test.wantError)
			}
			if test.command == "large-stdout" && len(stdout) != test.wantBytes {
				t.Fatalf("stdout bytes = %d, want %d", len(stdout), test.wantBytes)
			}
			if test.command == "large-stderr" && len(stderr) != test.wantBytes {
				t.Fatalf("stderr bytes = %d, want %d", len(stderr), test.wantBytes)
			}
		})
	}
}

func TestRunSSHCommandReturnsBoundedStderrForNonzeroExit(t *testing.T) {
	clientPrivateKey, clientSigner := newTestSigner(t)
	_, hostSigner := newTestSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())
	cfg := newTestSSHCommandConfig(t, server.Address(), clientPrivateKey, hostSigner.PublicKey())

	stdout, stderr, err := RunSSHCommand(context.Background(), cfg, "exit-42")
	if err == nil || !strings.Contains(err.Error(), "remote command failed with exit status 42: vnstat failed") {
		t.Fatalf("error = %v, want exit status and stderr", err)
	}
	if len(stdout) != 0 {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if got, want := string(stderr), "vnstat failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

type testSSHServer struct {
	listener            net.Listener
	serverConfig        *ssh.ServerConfig
	commands            chan string
	acceptedConnections atomic.Int64
	ptyRequests         atomic.Int64
	connectionsMu       sync.Mutex
	connections         map[net.Conn]struct{}
	waitGroup           sync.WaitGroup
}

func startTestSSHServer(t *testing.T, hostSigner ssh.Signer, allowedClientKey ssh.PublicKey) *testSSHServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if metadata.User() != "test-user" {
				return nil, fmt.Errorf("unexpected user %q", metadata.User())
			}
			if !bytes.Equal(key.Marshal(), allowedClientKey.Marshal()) {
				return nil, errors.New("unknown public key")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostSigner)

	server := &testSSHServer{
		listener:     listener,
		serverConfig: serverConfig,
		commands:     make(chan string, 16),
		connections:  make(map[net.Conn]struct{}),
	}
	server.waitGroup.Add(1)
	go server.acceptConnections()
	t.Cleanup(func() {
		_ = server.listener.Close()
		server.connectionsMu.Lock()
		for connection := range server.connections {
			_ = connection.Close()
		}
		server.connectionsMu.Unlock()
		server.waitGroup.Wait()
	})
	return server
}

func (server *testSSHServer) Address() string {
	return server.listener.Addr().String()
}

func (server *testSSHServer) acceptConnections() {
	defer server.waitGroup.Done()
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		server.acceptedConnections.Add(1)
		server.connectionsMu.Lock()
		server.connections[connection] = struct{}{}
		server.connectionsMu.Unlock()

		server.waitGroup.Add(1)
		go server.handleConnection(connection)
	}
}

func (server *testSSHServer) handleConnection(connection net.Conn) {
	defer server.waitGroup.Done()
	defer func() {
		_ = connection.Close()
		server.connectionsMu.Lock()
		delete(server.connections, connection)
		server.connectionsMu.Unlock()
	}()

	serverConnection, channels, requests, err := ssh.NewServerConn(connection, server.serverConfig)
	if err != nil {
		return
	}
	defer serverConnection.Close()
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		server.handleSession(channel, channelRequests)
	}
}

func (server *testSSHServer) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type == "pty-req" {
			server.ptyRequests.Add(1)
			_ = request.Reply(false, nil)
			continue
		}
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}

		var payload struct {
			Command string
		}
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			_ = request.Reply(false, nil)
			return
		}
		_ = request.Reply(true, nil)
		select {
		case server.commands <- payload.Command:
		default:
		}
		server.executeCommand(channel, payload.Command)
		return
	}
}

func (server *testSSHServer) executeCommand(channel ssh.Channel, command string) {
	switch command {
	case "success":
		_, _ = io.WriteString(channel, `{"ok":true}`)
		sendExitStatus(channel, 0)
	case "exit-42":
		_, _ = io.WriteString(channel.Stderr(), "vnstat failed\n")
		sendExitStatus(channel, 42)
	case "large-stdout":
		_, _ = channel.Write(bytes.Repeat([]byte("x"), int(DefaultSSHMaxOutputBytes)+1))
		sendExitStatus(channel, 0)
	case "large-stderr":
		_, _ = channel.Stderr().Write(bytes.Repeat([]byte("x"), int(DefaultSSHMaxStderrBytes)+1))
		sendExitStatus(channel, 1)
	case "hang":
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := channel.Write([]byte(".")); err != nil {
				return
			}
		}
	default:
		_, _ = io.WriteString(channel.Stderr(), "unknown test command\n")
		sendExitStatus(channel, 127)
	}
}

func sendExitStatus(channel ssh.Channel, status uint32) {
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
		Status uint32
	}{Status: status}))
}

type hangingTCPServer struct {
	listener      net.Listener
	connectionsMu sync.Mutex
	connections   map[net.Conn]struct{}
	waitGroup     sync.WaitGroup
}

func startHangingTCPServer(t *testing.T) *hangingTCPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	server := &hangingTCPServer{
		listener:    listener,
		connections: make(map[net.Conn]struct{}),
	}
	server.waitGroup.Add(1)
	go func() {
		defer server.waitGroup.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			server.connectionsMu.Lock()
			server.connections[connection] = struct{}{}
			server.connectionsMu.Unlock()
			server.waitGroup.Add(1)
			go func() {
				defer server.waitGroup.Done()
				_, _ = io.Copy(io.Discard, connection)
				_ = connection.Close()
				server.connectionsMu.Lock()
				delete(server.connections, connection)
				server.connectionsMu.Unlock()
			}()
		}
	}()
	t.Cleanup(func() {
		_ = server.listener.Close()
		server.connectionsMu.Lock()
		for connection := range server.connections {
			_ = connection.Close()
		}
		server.connectionsMu.Unlock()
		server.waitGroup.Wait()
	})
	return server
}

func (server *hangingTCPServer) Address() string {
	return server.listener.Addr().String()
}

func newTestSSHCommandConfig(
	t *testing.T,
	address string,
	clientPrivateKey ed25519.PrivateKey,
	hostPublicKey ssh.PublicKey,
) SSHCommandConfig {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q) error = %v", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("strconv.Atoi(%q) error = %v", portText, err)
	}
	return SSHCommandConfig{
		Host:           host,
		Port:           port,
		Username:       "test-user",
		PrivateKeyFile: writePrivateKeyFile(t, clientPrivateKey, nil),
		KnownHostsFile: writeKnownHostsFile(t, address, hostPublicKey),
		ConnectTimeout: time.Second,
		CommandTimeout: time.Second,
	}
}

func newTestSigner(t *testing.T) (ed25519.PrivateKey, ssh.Signer) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("ssh.NewSignerFromKey() error = %v", err)
	}
	return privateKey, signer
}

func writePrivateKeyFile(t *testing.T, privateKey ed25519.PrivateKey, passphrase []byte) string {
	t.Helper()
	var (
		block *pem.Block
		err   error
	)
	if passphrase == nil {
		block, err = ssh.MarshalPrivateKey(privateKey, "test-key")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test-key", passphrase)
	}
	if err != nil {
		t.Fatalf("marshal private key error = %v", err)
	}
	return writeTestFile(t, "id_ed25519", pem.EncodeToMemory(block))
}

func writeKnownHostsFile(t *testing.T, address string, publicKey ssh.PublicKey) string {
	t.Helper()
	contents := []byte{}
	if publicKey != nil {
		contents = []byte(knownhosts.Line([]string{address}, publicKey) + "\n")
	}
	return writeTestFile(t, "known_hosts", contents)
}

func writeTestFile(t *testing.T, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
	return path
}
