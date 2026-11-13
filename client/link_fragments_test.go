// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"encoding/json"
	"os"
	"testing"
)

// linkFragments is testdata/link-fragments.json, which usechirp.app's link
// page is tested against too.
type linkFragments struct {
	Page  string `json:"page"`
	Cases []struct {
		Fragment string `json:"fragment"`
		Link     bool   `json:"link"`
		Why      string `json:"why"`
	} `json:"cases"`
}

func readLinkFragments(t *testing.T) linkFragments {
	t.Helper()
	data, err := os.ReadFile("testdata/link-fragments.json")
	if err != nil {
		t.Fatal(err)
	}
	var v linkFragments
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) == 0 {
		t.Fatal("testdata/link-fragments.json has no cases")
	}
	return v
}

func TestLinksAreReadAsTheLinkPageShowsThem(t *testing.T) {
	v := readLinkFragments(t)
	for _, c := range v.Cases {
		target, err := ParseTarget(v.Page + c.Fragment)
		if isLink := err == nil && target.Token != ""; isLink != c.Link {
			t.Errorf("%s (%q): read as a link %v, want %v", c.Why, c.Fragment, isLink, c.Link)
		}
	}
}

func TestTheLinkChirpSendsIsOneThePageShows(t *testing.T) {
	v := readLinkFragments(t)
	sent := Peer{Link: v.Cases[0].Fragment, LinkSecret: "0042"}
	for _, c := range v.Cases {
		if c.Fragment == sent.LinkFragment() {
			if !c.Link {
				t.Fatalf("Chirp sends %q, which the vector says is not a link", c.Fragment)
			}
			return
		}
	}
	t.Fatalf("the vector has no case for the fragment Chirp sends, %q", sent.LinkFragment())
}
