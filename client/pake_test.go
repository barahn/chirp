// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"strconv"
	"testing"
)

func TestTheSpokenCodeAndLinkCarryTheSecret(t *testing.T) {
	p := Peer{Formatted: "482-913", Link: "tok", Secret: "5071", LinkSecret: "0042"}
	if got := p.Spoken(); got != "482-913-5071" {
		t.Errorf("Spoken = %q", got)
	}
	if got := p.LinkFragment(); got != "tok.0042" {
		t.Errorf("LinkFragment = %q", got)
	}
	plain := Peer{Formatted: "482-913", Link: "tok"}
	if plain.Spoken() != "482-913" || plain.LinkFragment() != "tok" {
		t.Errorf("without secrets: %q, %q", plain.Spoken(), plain.LinkFragment())
	}
}

func TestEachCodeHasItsOwnSecretUsedOnce(t *testing.T) {
	b := newSecretBook()
	s := b.forCode("111111")
	if len(s) != SecretLength || b.forCode("111111") != s {
		t.Fatalf("forCode gave %q, then %q", s, b.forCode("111111"))
	}
	if got, ok := b.take("111111"); !ok || got != s {
		t.Fatalf("take = %q, %v", got, ok)
	}
	if _, ok := b.take("111111"); ok {
		t.Error("a secret was taken twice")
	}
	for i := 0; i < maxCodeSecrets+3; i++ {
		b.forCode(strconv.Itoa(200000 + i))
	}
	if len(b.codes) != maxCodeSecrets {
		t.Errorf("the book keeps %d secrets, want %d", len(b.codes), maxCodeSecrets)
	}
	if _, ok := b.take("200000"); ok {
		t.Error("the oldest secret was kept past the bound")
	}
}

func TestSecretsAreFourDigits(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		s := newSecret()
		if !isSecret(s) {
			t.Fatalf("newSecret() = %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 150 {
		t.Errorf("200 secrets had only %d distinct values", len(seen))
	}
}
