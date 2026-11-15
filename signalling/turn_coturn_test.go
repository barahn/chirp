// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/pion/turn/v5"
)

// These tests hold the relay's credentials to coturn itself, the TURN server
// deploy/ runs, rather than to pion's: the two differ on what an expired
// credential may still do (docs/network-paths-proposal.md, part 1).
//
// They run coturn's turnserver from $CHIRP_COTURN, or from PATH. Without one
// they are skipped, unless $CHIRP_REQUIRE_COTURN is set, as CI sets it.

const coturnRealm = "chirp.test"

// coturnBinary finds turnserver, or skips the test, or fails it where coturn
// is required.
func coturnBinary(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("CHIRP_COTURN"); bin != "" {
		return bin
	}
	if bin, err := exec.LookPath("turnserver"); err == nil {
		return bin
	}
	if os.Getenv("CHIRP_REQUIRE_COTURN") != "" {
		t.Fatal("coturn is required: set CHIRP_COTURN or put turnserver on PATH")
	}
	t.Skip("no coturn: set CHIRP_COTURN or put turnserver on PATH")
	return ""
}

func startCoturn(t *testing.T, secret string) string {
	t.Helper()
	bin := coturnBinary(t)
	port := freeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin,
		"-n", "--no-cli", "--no-tls", "--no-dtls", "--fingerprint",
		"--listening-ip=127.0.0.1", "--listening-port="+strconv.Itoa(port),
		"--relay-ip=127.0.0.1", "--min-port=49160", "--max-port=49200",
		"--allow-loopback-peers",
		"--use-auth-secret", "--static-auth-secret="+secret, "--realm="+coturnRealm,
		"--log-file=stdout", "--simple-log",
	)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	logs, err := os.CreateTemp(t.TempDir(), "coturn-*.log")
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start coturn: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			if b, err := os.ReadFile(logs.Name()); err == nil {
				t.Logf("coturn log:\n%s", b)
			}
		}
	})
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	waitForSTUN(t, addr)
	return addr
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func waitForSTUN(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		c, err := turnClient(addr, "", "")
		if err == nil {
			_, err = c.SendBindingRequest()
			c.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("coturn at %s did not answer STUN", addr)
}

func turnClient(addr, user, pass string) (*turn.Client, error) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: addr,
		TURNServerAddr: addr,
		Username:       user,
		Password:       pass,
		Realm:          coturnRealm,
		Conn:           conn,
		RTO:            200 * time.Millisecond,
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := c.Listen(); err != nil {
		c.Close()
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

func allocate(t *testing.T, addr string, cred turnCredential) (net.PacketConn, error) {
	t.Helper()
	c, err := turnClient(addr, cred.Username, cred.Credential)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.AllocateWithContext(ctx)
}

// relays reports whether a packet written to a fresh peer through relay
// arrives. A fresh peer makes the client ask coturn for a new permission,
// which coturn authenticates.
func relays(t *testing.T, relay net.PacketConn) error {
	t.Helper()
	peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Close() }()
	want := []byte("through coturn")
	if _, err := relay.WriteTo(want, peer.LocalAddr()); err != nil {
		return fmt.Errorf("write through the relay: %w", err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, 64)
	n, _, err := peer.ReadFrom(got)
	if err != nil {
		return fmt.Errorf("the peer got nothing: %w", err)
	}
	if string(got[:n]) != string(want) {
		return fmt.Errorf("the peer got %q, want %q", got[:n], want)
	}
	return nil
}

func TestCoturnAcceptsAFreshCredential(t *testing.T) {
	addr := startCoturn(t, testTURNSecret)
	cred, err := (&TURN{Secret: testTURNSecret}).credential()
	if err != nil {
		t.Fatal(err)
	}
	relay, err := allocate(t, addr, cred)
	if err != nil {
		t.Fatalf("allocate with a fresh credential: %v", err)
	}
	defer func() { _ = relay.Close() }()
	if err := relays(t, relay); err != nil {
		t.Fatal(err)
	}
}

func TestCoturnRefusesAnExpiredCredential(t *testing.T) {
	addr := startCoturn(t, testTURNSecret)
	expired := &TURN{Secret: testTURNSecret, now: func() time.Time { return time.Now().Add(-2 * DefaultTURNLifetime) }}
	cred, err := expired.credential()
	if err != nil {
		t.Fatal(err)
	}
	if relay, err := allocate(t, addr, cred); err == nil {
		_ = relay.Close()
		t.Fatal("coturn allocated with a credential that had expired")
	}
}

func TestCoturnRefusesACredentialUnderAnotherSecret(t *testing.T) {
	addr := startCoturn(t, testTURNSecret)
	cred, err := (&TURN{Secret: "not the shared secret"}).credential()
	if err != nil {
		t.Fatal(err)
	}
	if relay, err := allocate(t, addr, cred); err == nil {
		_ = relay.Close()
		t.Fatal("coturn allocated with a credential under the wrong secret")
	}
}

// A session outlives its credential: coturn checks the expiry only on an
// allocation's first authenticated request, and keeps the key for the rest.
func TestCoturnKeepsRelayingAfterTheCredentialExpires(t *testing.T) {
	addr := startCoturn(t, testTURNSecret)
	short := &TURN{Secret: testTURNSecret, Lifetime: 2 * time.Second}
	cred, err := short.credential()
	if err != nil {
		t.Fatal(err)
	}
	relay, err := allocate(t, addr, cred)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	defer func() { _ = relay.Close() }()
	time.Sleep(3 * time.Second)
	if err := relays(t, relay); err != nil {
		t.Fatalf("after the credential expired: %v", err)
	}
}
