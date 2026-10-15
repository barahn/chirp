// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ProbeTimeout is how long a server has to answer a probe.
const ProbeTimeout = 5 * time.Second

// Why a probe failed. They say which layer gave out, nothing more: the page
// shows them, and a server's error text is not ours to republish.
const (
	ErrTimeout = "timeout"
	ErrDNS     = "dns"
	ErrConnect = "connect"
	ErrTLS     = "tls"
	ErrStatus  = "status"
	ErrOther   = "other"
)

// Prober asks servers whether they are up.
type Prober struct {
	// Client makes the requests. Nil means a client that opens a new
	// connection for every probe, so each one measures reaching the server
	// from nothing, TLS handshake included, and that follows no redirect.
	Client *http.Client
}

func newClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.Proxy = nil
	return &http.Client{
		Transport: transport,
		Timeout:   ProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Probe asks the node at base for /healthz. It is up if it answers 200
// within ProbeTimeout. The sample's At and Vantage are the caller's to set.
func (p *Prober) Probe(ctx context.Context, base string) Sample {
	client := p.Client
	if client == nil {
		client = newClient()
	}
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/healthz", nil)
	if err != nil {
		return Sample{Err: ErrOther}
	}
	req.Header.Set("User-Agent", "chirp-status (+https://usechirp.app/status/)")
	resp, err := client.Do(req)
	if err != nil {
		return Sample{Latency: time.Since(start), Err: classify(err)}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
	latency := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		return Sample{Latency: latency, Err: ErrStatus}
	}
	return Sample{OK: true, Latency: latency}
}

func classify(err error) string {
	var (
		dnsErr   *net.DNSError
		netErr   net.Error
		opErr    *net.OpError
		certErr  *tls.CertificateVerificationError
		alertErr tls.AlertError
		recErr   tls.RecordHeaderError
		unknown  x509.UnknownAuthorityError
		hostErr  x509.HostnameError
		invalid  x509.CertificateInvalidError
	)
	switch {
	case errors.As(err, &dnsErr):
		return ErrDNS
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return ErrTimeout
	case errors.As(err, &certErr), errors.As(err, &alertErr), errors.As(err, &recErr),
		errors.As(err, &unknown), errors.As(err, &hostErr), errors.As(err, &invalid):
		return ErrTLS
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return ErrConnect
	}
	return ErrOther
}
