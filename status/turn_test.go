// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- matching coturn's TURN REST HMAC-SHA1 format
	"encoding/base64"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/turn/v5"
)

const testTURNSecret = "status-prober-turn-secret"

func startInProcessTURN(t *testing.T, auth turn.AuthHandler) string {
	t.Helper()
	udpConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := udpConn.LocalAddr().(*net.UDPAddr).Port
	tcpLn, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		_ = udpConn.Close()
		t.Fatalf("listen tcp on udp port %d: %v", port, err)
	}

	gen := &turn.RelayAddressGeneratorStatic{
		RelayAddress: net.ParseIP("127.0.0.1"),
		Address:      "127.0.0.1",
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             defaultTURNRealm,
		AuthHandler:       auth,
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udpConn, RelayAddressGenerator: gen}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: tcpLn, RelayAddressGenerator: gen}},
	})
	if err != nil {
		_ = udpConn.Close()
		_ = tcpLn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return udpConn.LocalAddr().String()
}

// coturnLikeAuth checks the HMAC-SHA1 and expiry only the first time a
// username is seen, mimicking coturn's behaviour of keeping the allocation key
// valid after the credential expires.
func coturnLikeAuth(secret string) turn.AuthHandler {
	var mu sync.Mutex
	seen := make(map[string][]byte)
	return func(ra *turn.RequestAttributes) (string, []byte, bool) {
		mu.Lock()
		defer mu.Unlock()
		if key, ok := seen[ra.Username]; ok {
			return ra.Username, key, true
		}
		ts, _, ok := strings.Cut(ra.Username, ":")
		if !ok {
			return "", nil, false
		}
		exp, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return "", nil, false
		}
		if time.Now().Unix() > exp {
			return "", nil, false
		}
		mac := hmac.New(sha1.New, []byte(secret))
		_, _ = mac.Write([]byte(ra.Username))
		pass := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		key := turn.GenerateAuthKey(ra.Username, ra.Realm, pass)
		seen[ra.Username] = key
		return ra.Username, key, true
	}
}

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

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func startCoturn(t *testing.T, secret string) string {
	t.Helper()
	bin := coturnBinary(t)
	port := freeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin,
		"-n", "--no-cli", "--no-tls", "--no-dtls", "--fingerprint",
		"--listening-ip=127.0.0.1", "--listening-port="+strconv.Itoa(port),
		"--relay-ip=127.0.0.1", "--min-port=49201", "--max-port=49240",
		"--allow-loopback-peers",
		"--use-auth-secret", "--static-auth-secret="+secret, "--realm="+defaultTURNRealm,
		"--log-file=stdout", "--simple-log",
	)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start coturn: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	waitForSTUNReady(t, addr)
	return addr
}

func waitForSTUNReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if err := CheckSTUN(context.Background(), addr); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("coturn at %s did not answer STUN", addr)
}

func TestCheckSTUN(t *testing.T) {
	if err := CheckSTUN(context.Background(), ""); err == nil {
		t.Fatal("empty address accepted")
	}
	addr := startInProcessTURN(t, turn.LongTermTURNRESTAuthHandler(testTURNSecret, nil))
	if err := CheckSTUN(context.Background(), addr); err != nil {
		t.Fatalf("CheckSTUN: %v", err)
	}
}

func TestCheckTURNValidation(t *testing.T) {
	ctx := context.Background()
	if err := CheckTURN(ctx, TURNConfig{Secret: testTURNSecret}); err == nil {
		t.Error("empty Addr accepted")
	}
	if err := CheckTURN(ctx, TURNConfig{Addr: "127.0.0.1:3478"}); err == nil {
		t.Error("empty Secret accepted")
	}
}

func TestCheckTURNFailsWhenServerEnforcesExpiryOnRelay(t *testing.T) {
	// pion/turn's LongTermTURNRESTAuthHandler re-checks expiry on CreatePermission,
	// so relaying after expiry must fail the conformance check.
	addr := startInProcessTURN(t, turn.LongTermTURNRESTAuthHandler(testTURNSecret, nil))
	err := CheckTURN(context.Background(), TURNConfig{
		Addr:     addr,
		Secret:   testTURNSecret,
		Lifetime: 1 * time.Second,
		Grace:    1 * time.Second,
		Timeout:  2 * time.Second,
	})
	if err == nil {
		t.Fatal("CheckTURN succeeded against a server that rejects expired credentials on CreatePermission")
	}
}

func TestCheckTURNSucceedsWhenServerKeepsRelayingAfterExpiry(t *testing.T) {
	addr := startInProcessTURN(t, coturnLikeAuth(testTURNSecret))
	cfg := TURNConfig{
		Addr:     addr,
		Secret:   testTURNSecret,
		Lifetime: 1 * time.Second,
		Grace:    1 * time.Second,
		Timeout:  3 * time.Second,
	}
	if err := CheckTURN(context.Background(), cfg); err != nil {
		t.Fatalf("CheckTURN: %v", err)
	}
	s := (&Prober{}).ProbeTURN(context.Background(), cfg)
	if !s.OK {
		t.Errorf("ProbeTURN OK = false, err = %q", s.Err)
	}
	if s.Err != "" {
		t.Errorf("ProbeTURN Err = %q, want empty", s.Err)
	}
	if s.Latency <= 0 {
		t.Errorf("ProbeTURN Latency = %v, want > 0", s.Latency)
	}
}

func TestCheckTURNAgainstCoturn(t *testing.T) {
	addr := startCoturn(t, testTURNSecret)
	if err := CheckSTUN(context.Background(), addr); err != nil {
		t.Fatalf("CheckSTUN against coturn: %v", err)
	}
	if err := CheckTURN(context.Background(), TURNConfig{
		Addr:     addr,
		Secret:   testTURNSecret,
		Lifetime: 2 * time.Second,
		Grace:    1 * time.Second,
	}); err != nil {
		t.Fatalf("CheckTURN against coturn: %v", err)
	}
}
