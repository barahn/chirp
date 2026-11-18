// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- the TURN REST credential format coturn checks is HMAC-SHA1
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/pion/turn/v5"
)

const (
	defaultTURNRealm    = "chirp"
	defaultTURNLifetime = 2 * time.Second
	defaultTURNGrace    = 1 * time.Second
)

// TURNConfig configures a TURN conformance check.
//
// The check allocates a relay with a short-lived TURN REST credential, waits
// past its expiry, and sends a packet to a fresh peer through the allocation.
// coturn checks the credential expiry only on the initial allocation, so it
// keeps relaying; servers that re-check expiry on CreatePermission or Refresh
// fail the check (docs/network-paths-proposal.md).
type TURNConfig struct {
	// Addr is the TURN server's host:port.
	Addr string
	// Secret is the shared static-auth-secret used to mint the credential.
	Secret string
	// Realm is the TURN realm; empty defaults to "chirp".
	Realm string
	// Lifetime is how long the minted credential is valid; non-positive
	// defaults to 2s.
	Lifetime time.Duration
	// Grace is how long past expiry to wait before relaying; non-positive
	// defaults to 1s.
	Grace time.Duration
	// Timeout bounds each network step (STUN, Allocate, Read); non-positive
	// defaults to ProbeTimeout.
	Timeout time.Duration
	// PeerAddr, if set, is the UDP address the allocation relays a probe
	// packet to. When nil, a local UDP socket is opened and the relayed
	// packet is read back on it.
	PeerAddr net.Addr
}

func (c TURNConfig) realm() string {
	if c.Realm != "" {
		return c.Realm
	}
	return defaultTURNRealm
}

func (c TURNConfig) lifetime() time.Duration {
	if c.Lifetime > 0 {
		return c.Lifetime
	}
	return defaultTURNLifetime
}

func (c TURNConfig) grace() time.Duration {
	if c.Grace > 0 {
		return c.Grace
	}
	return defaultTURNGrace
}

func (c TURNConfig) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return ProbeTimeout
}

// CheckSTUN verifies that addr answers a STUN Binding Request over both UDP
// and TCP (docs/status-page-proposal.md, Conformance).
func CheckSTUN(ctx context.Context, addr string) error {
	if addr == "" {
		return errors.New("stun: empty address")
	}
	if err := checkSTUNUDP(ctx, addr); err != nil {
		return fmt.Errorf("stun udp: %w", err)
	}
	if err := checkSTUNTCP(ctx, addr); err != nil {
		return fmt.Errorf("stun tcp: %w", err)
	}
	return nil
}

func checkSTUNUDP(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	var lc net.ListenConfig
	conn, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: addr,
		Conn:           conn,
		RTO:            200 * time.Millisecond,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.SendBindingRequest()
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func checkSTUNTCP(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	var d net.Dialer
	tcpConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = tcpConn.Close() }()

	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: addr,
		Conn:           turn.NewSTUNConn(tcpConn),
		RTO:            200 * time.Millisecond,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.SendBindingRequest()
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// CheckTURN allocates a relay on cfg.Addr using a short-lived TURN REST
// credential, waits until after the credential has expired, and verifies that
// the allocation still relays a packet to a fresh peer.
func CheckTURN(ctx context.Context, cfg TURNConfig) error {
	if cfg.Addr == "" {
		return errors.New("turn: empty address")
	}
	if cfg.Secret == "" {
		return errors.New("turn: empty secret")
	}

	lifetime := cfg.lifetime()
	user, pass, err := turnRESTCredential(cfg.Secret, time.Now().Add(lifetime))
	if err != nil {
		return fmt.Errorf("turn: mint credential: %w", err)
	}

	var lc net.ListenConfig
	conn, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: cfg.Addr,
		TURNServerAddr: cfg.Addr,
		Username:       user,
		Password:       pass,
		Realm:          cfg.realm(),
		Conn:           conn,
		RTO:            200 * time.Millisecond,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		return err
	}

	timeout := cfg.timeout()
	allocCtx, allocCancel := context.WithTimeout(ctx, timeout)
	relay, err := c.AllocateWithContext(allocCtx)
	allocCancel()
	if err != nil {
		return fmt.Errorf("turn allocate: %w", err)
	}
	defer func() { _ = relay.Close() }()

	timer := time.NewTimer(lifetime + cfg.grace())
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	return relayProbe(ctx, relay, cfg.PeerAddr, timeout)
}

func relayProbe(ctx context.Context, relay net.PacketConn, peerAddr net.Addr, timeout time.Duration) error {
	var localPeer net.PacketConn
	if peerAddr == nil {
		var lc net.ListenConfig
		var err error
		localPeer, err = lc.ListenPacket(ctx, "udp4", "127.0.0.1:0")
		if err != nil {
			return err
		}
		defer func() { _ = localPeer.Close() }()
		peerAddr = localPeer.LocalAddr()
	}

	want := []byte("chirp-turn-probe")
	if _, err := relay.WriteTo(want, peerAddr); err != nil {
		return fmt.Errorf("turn relay write after expiry: %w", err)
	}
	if localPeer == nil {
		return nil
	}

	_ = localPeer.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 64)
	n, _, err := localPeer.ReadFrom(buf)
	if err != nil {
		return fmt.Errorf("turn relay read after expiry: %w", err)
	}
	if string(buf[:n]) != string(want) {
		return fmt.Errorf("turn relay payload = %q, want %q", buf[:n], want)
	}
	return nil
}

// ProbeTURN runs CheckTURN and returns the outcome as a Sample.
func (p *Prober) ProbeTURN(ctx context.Context, cfg TURNConfig) Sample {
	start := time.Now()
	err := CheckTURN(ctx, cfg)
	latency := time.Since(start)
	if err != nil {
		return Sample{Latency: latency, Err: classify(err)}
	}
	return Sample{OK: true, Latency: latency}
}

func turnRESTCredential(secret string, expiresAt time.Time) (user, pass string, err error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", "", err
	}
	user = strconv.FormatInt(expiresAt.Unix(), 10) + ":" + hex.EncodeToString(id)
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(user))
	pass = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return user, pass, nil
}
