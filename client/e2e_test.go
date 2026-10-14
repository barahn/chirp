// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Two separate processes, one for each side, find each other through a relay
// and reach the consent request (barahn/chirp#10's "done when"); the owner
// lets the other in, and the controller sees it (barahn/chirp#11).
//
// The processes are this test binary run again in a role, the usual Go way of
// testing a program's processes. They run the code cmd/chirp runs -- Owner and
// Help -- without its window, which needs a display and cgo that a test run
// does not have.

const (
	roleEnv  = "CHIRP_E2E_ROLE"
	relayEnv = "CHIRP_E2E_RELAY"
	codeEnv  = "CHIRP_E2E_CODE"
)

func TestMain(m *testing.M) {
	switch os.Getenv(roleEnv) {
	case "owner":
		os.Exit(ownerProcess())
	case "controller":
		os.Exit(controllerProcess())
	}
	os.Exit(m.Run())
}

// ownerProcess prints its code, then the consent request it receives, and
// answers it with Allow, as its user would in the prompt. It keeps running
// until it is stopped, so the session it allowed stays up.
func ownerProcess() int {
	c, err := New(os.Getenv(relayEnv))
	if err != nil {
		fmt.Println("ERROR", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	printed := false
	var o *Owner
	o = NewOwner(c, "", func(ev OwnerEvent) {
		switch {
		case ev.Consent != nil:
			fmt.Println("CONSENT", ev.Consent.Name, ev.Consent.NameVerified)
			req := ev.Consent
			go func() {
				if _, err := o.Answer(ctx, req.SessionID, true); err != nil {
					fmt.Println("ERROR", err)
					return
				}
				fmt.Println("ANSWERED")
			}()
		case ev.Phase == OwnerReady && !printed:
			fmt.Println("CODE", ev.Peer.Formatted)
			printed = true
		}
	})
	_ = o.Run(ctx)
	return 0
}

// controllerProcess types the code it is given, prints when the other side is
// being asked, and again when it is let in.
func controllerProcess() int {
	c, err := New(os.Getenv(relayEnv))
	if err != nil {
		fmt.Println("ERROR", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	status := 1
	asked := false
	c.Help(ctx, os.Getenv(codeEnv), "Ana", func(ev HelperEvent) {
		switch ev.Phase {
		case HelperAsking:
			if !asked {
				fmt.Println("ASKING")
				asked = true
			}
		case HelperAllowed:
			fmt.Println("ALLOWED")
			status = 0
			cancel()
		case HelperFailed:
			fmt.Println("FAILED", ev.Err)
			cancel()
		}
	})
	return status
}

// process is a running role, read line by line.
type process struct {
	cmd   *exec.Cmd
	lines chan string
}

func start(t *testing.T, role string, env ...string) *process {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(append(os.Environ(), roleEnv+"="+role), env...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &process{cmd: cmd, lines: make(chan string, 16)}
	go func() {
		defer close(p.lines)
		s := bufio.NewScanner(out)
		for s.Scan() {
			p.lines <- s.Text()
		}
		_, _ = io.Copy(io.Discard, out)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return p
}

// expect returns the first line that starts with prefix.
func (p *process) expect(t *testing.T, prefix string) string {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				t.Fatalf("process ended before printing %q", prefix)
			}
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
			t.Logf("process: %s", line)
		case <-deadline:
			t.Fatalf("timed out waiting for %q", prefix)
		}
	}
}

func TestTwoProcessesReachConsentAndTheAnswer(t *testing.T) {
	if testing.Short() {
		t.Skip("starts two processes")
	}
	r := newRelay(t)

	owner := start(t, "owner", relayEnv+"="+r.URL)
	code := owner.expect(t, "CODE")

	controller := start(t, "controller", relayEnv+"="+r.URL, codeEnv+"="+code)
	controller.expect(t, "ASKING")

	if got := owner.expect(t, "CONSENT"); got != "Ana false" {
		t.Errorf("the owner was asked by %q, want the name Ana, unverified", got)
	}
	owner.expect(t, "ANSWERED")
	controller.expect(t, "ALLOWED")
}
