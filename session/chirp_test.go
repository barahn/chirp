// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package chirp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

func TestChirp_CodeGenerationAndFormatting(t *testing.T) {
	code, err := chirp.GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	if len(code) != 6 {
		t.Fatalf("Expected 6 digits, got %d (%s)", len(code), code)
	}

	formatted := chirp.FormatCode(code)
	if len(formatted) != 7 || formatted[3] != '-' {
		t.Fatalf("Expected formatted code XXX-XXX, got %s", formatted)
	}

	normalized, err := chirp.NormalizeCode(formatted)
	if err != nil {
		t.Fatalf("NormalizeCode failed: %v", err)
	}
	if normalized != code {
		t.Fatalf("Expected normalized %s, got %s", code, normalized)
	}

	// Test variations
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"123-456", "123456", false},
		{" 123 456 ", "123456", false},
		{"123.456", "123456", false},
		{"12345", "", true},   // Too short
		{"1234567", "", true}, // Too long
		{"12345a", "", true},  // Non-digit
		{"", "", true},        // Empty
	}

	for _, tc := range tests {
		got, err := chirp.NormalizeCode(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("NormalizeCode(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestChirp_SessionLifecycleFlow(t *testing.T) {
	sm := chirp.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	// 1. Create Session
	sess, err := sm.CreateSession("tech-1", "Alice Technician", 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sess.State != chirp.StatePending {
		t.Fatalf("Expected StatePending, got %s", sess.State)
	}
	if sess.TechnicianName != "Alice Technician" {
		t.Fatalf("TechnicianName mismatch: %s", sess.TechnicianName)
	}

	// Retrieve by ID
	sByID, err := sm.GetSession(sess.ID)
	if err != nil || sByID.ID != sess.ID {
		t.Fatalf("GetSession by ID failed: %v", err)
	}

	// Retrieve by Code
	sByCode, err := sm.GetSessionByCode(sess.Code)
	if err != nil || sByCode.ID != sess.ID {
		t.Fatalf("GetSessionByCode failed: %v", err)
	}

	// Retrieve by formatted code
	sByFmtCode, err := sm.GetSessionByCode(sess.FormattedCode)
	if err != nil || sByFmtCode.ID != sess.ID {
		t.Fatalf("GetSessionByCode (formatted) failed: %v", err)
	}

	// 2. Connect Client
	clientInfo := &chirp.ClientInfo{
		Hostname: "workstation-alpha",
		OS:       "Linux Ubuntu 24.04",
		Arch:     "x86_64",
		RemoteIP: "192.168.1.100",
	}
	connected, err := sm.ConnectClient(sess.Code, clientInfo)
	if err != nil {
		t.Fatalf("ConnectClient failed: %v", err)
	}
	if connected.State != chirp.StateConnected {
		t.Fatalf("Expected StateConnected, got %s", connected.State)
	}
	if connected.ClientInfo.Hostname != "workstation-alpha" {
		t.Fatalf("Client hostname mismatch: %s", connected.ClientInfo.Hostname)
	}

	// 3. Request Consent
	consentReq, err := sm.RequestConsent(sess.Code)
	if err != nil {
		t.Fatalf("RequestConsent failed: %v", err)
	}
	if consentReq.State != chirp.StateConsentRequested {
		t.Fatalf("Expected StateConsentRequested, got %s", consentReq.State)
	}

	// 4. Submit Consent (Granted)
	consentRes, err := sm.SubmitConsent(sess.Code, true, "")
	if err != nil {
		t.Fatalf("SubmitConsent failed: %v", err)
	}
	if consentRes.State != chirp.StateConsentGranted {
		t.Fatalf("Expected StateConsentGranted, got %s", consentRes.State)
	}
	if !consentRes.Consent.Granted {
		t.Fatalf("Expected Consent.Granted == true")
	}

	// 5. Activate Session
	activeSess, err := sm.ActivateSession(sess.Code)
	if err != nil {
		t.Fatalf("ActivateSession failed: %v", err)
	}
	if activeSess.State != chirp.StateActive {
		t.Fatalf("Expected StateActive, got %s", activeSess.State)
	}

	// List Active Sessions
	activeList := sm.ListActiveSessions()
	if len(activeList) != 1 {
		t.Fatalf("Expected 1 active session, got %d", len(activeList))
	}

	// 6. Terminate Session
	endedSess, err := sm.EndSession(sess.ID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}
	if endedSess.State != chirp.StateTerminated {
		t.Fatalf("Expected StateTerminated, got %s", endedSess.State)
	}

	// Verify Active List is now empty
	if len(sm.ListActiveSessions()) != 0 {
		t.Fatalf("Expected 0 active sessions after termination")
	}
}

func TestChirp_ConsentDeniedFlow(t *testing.T) {
	sm := chirp.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	sess, _ := sm.CreateSession("tech-2", "Bob Tech", 5*time.Minute)
	_, _ = sm.ConnectClient(sess.Code, nil)
	_, _ = sm.RequestConsent(sess.Code)

	denied, err := sm.SubmitConsent(sess.Code, false, "User cancelled request")
	if err != nil {
		t.Fatalf("SubmitConsent failed: %v", err)
	}
	if denied.State != chirp.StateConsentDenied {
		t.Fatalf("Expected StateConsentDenied, got %s", denied.State)
	}
	if denied.Consent.Granted != false {
		t.Fatalf("Expected Consent.Granted == false")
	}

	// Activate should fail on denied consent
	_, err = sm.ActivateSession(sess.Code)
	if err == nil {
		t.Fatalf("Expected error when activating a denied session, got nil")
	}
}

func TestChirp_WaitForConsent(t *testing.T) {
	sm := chirp.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	sess, _ := sm.CreateSession("tech-3", "Charlie", 5*time.Minute)
	_, _ = sm.ConnectClient(sess.Code, nil)
	_, _ = sm.RequestConsent(sess.Code)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = sm.SubmitConsent(sess.Code, true, "")
	}()

	granted, err := sm.WaitForConsent(ctx, sess.Code)
	if err != nil {
		t.Fatalf("WaitForConsent failed: %v", err)
	}
	if !granted {
		t.Fatalf("Expected granted == true")
	}
}

func TestChirp_ExpirationTTL(t *testing.T) {
	sm := chirp.NewSessionManager(50 * time.Millisecond)
	defer sm.Close()

	sess, err := sm.CreateSession("tech-4", "Dave", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	time.Sleep(80 * time.Millisecond)

	// Connect on expired session should return ErrSessionExpired
	_, err = sm.ConnectClient(sess.Code, nil)
	if err != chirp.ErrSessionExpired {
		t.Fatalf("Expected ErrSessionExpired, got %v", err)
	}
}

func TestChirp_SubscriptionEvents(t *testing.T) {
	sm := chirp.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	sess, _ := sm.CreateSession("tech-5", "Eve", 5*time.Minute)

	ch, unsub := sm.Subscribe(sess.Code)
	defer unsub()

	// Initial receive
	select {
	case s := <-ch:
		if s.State != chirp.StatePending {
			t.Fatalf("Expected initial state StatePending, got %s", s.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Timeout waiting for initial subscription event")
	}

	// Trigger connection update
	go func() {
		_, _ = sm.ConnectClient(sess.Code, &chirp.ClientInfo{Hostname: "host-1"})
	}()

	select {
	case s := <-ch:
		if s.State != chirp.StateConnected {
			t.Fatalf("Expected update state StateConnected, got %s", s.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Timeout waiting for connection event")
	}
}

func TestChirp_ConcurrentAccess(t *testing.T) {
	sm := chirp.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess, err := sm.CreateSession("tech", "Concurrent Tech", time.Minute)
			if err != nil {
				t.Errorf("Concurrent CreateSession failed: %v", err)
				return
			}
			_, _ = sm.GetSession(sess.ID)
			_, _ = sm.GetSessionByCode(sess.Code)
			_, _ = sm.ConnectClient(sess.Code, nil)
			_, _ = sm.EndSession(sess.Code)
		}(i)
	}
	wg.Wait()
}
