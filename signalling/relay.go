// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"encoding/json"
	"strings"

	"github.com/barahn/remotekit/bark"
)

// The relay's own vocabulary: messages the hub sends a peer itself, as opposed
// to the signals it forwards from the other end unread.
//
// Every relay message carries a "type" in the chirp. namespace. The "type"
// field is shared with the signals the peers exchange, whose values remotekit's
// webrtc.SignalType owns: "offer", "answer", "candidate", "close". A bare word
// here could collide with a type remotekit adds later, and a peer would then
// read a relay notice as a signal from the other end. The prefix keeps the
// relay's messages in a namespace remotekit's cannot reach, and lets a client
// tell the two apart without knowing every type.
const (
	// RelayErrorType reports that something the peer asked of the relay
	// failed, such as forwarding while the other end is absent.
	RelayErrorType = "chirp.error"

	// PeerJoinedType tells a peer that the other end of its session is present
	// and registered, naming that end's role. Both peers receive it when the
	// second arrives.
	//
	// It is what a client waits for before sending. A client's Dial returns
	// when the server writes the upgrade response, and the server registers the
	// peer only afterwards, so a message sent into that window is refused with
	// ErrNoPeer. Without this notice the sender could only retry blind; with
	// it, receiving the notice is proof the other end can be reached.
	PeerJoinedType = "chirp.peer_joined"

	// PeerLeftType tells a peer that the other end has gone, naming its role.
	// It is not sent when the hub itself closes.
	PeerLeftType = "chirp.peer_left"

	// ConsentRequestType is sent to the owner when a controller has joined its
	// session: the owner's Chirp shows the consent prompt from it. Its
	// "request" field is remotekit's bark.ConsentRequestPayload.
	//
	// The payload's technician_name is the name the controller typed. Nobody
	// registers, so it is a claim, and "name_verified" is always false to say
	// so on the wire as well as in the prompt.
	ConsentRequestType = "chirp.consent_request"
)

// relayTyped reports whether data carries a type in the relay's namespace. It
// reads the type with the same decoder a client does (client.decode), and
// with fewer fields to fail on, so anything a client would read as a relay
// message, this reads as one too.
func relayTyped(data []byte) bool {
	var head struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(data, &head) == nil && strings.HasPrefix(head.Type, "chirp.")
}

// encodeError builds a relay error message for err.
func encodeError(err error) []byte {
	b, _ := json.Marshal(map[string]string{"type": RelayErrorType, "error": err.Error()})
	return b
}

// encodeNotice builds a presence notice about the peer in role.
func encodeNotice(kind string, role Role) []byte {
	b, _ := json.Marshal(map[string]string{"type": kind, "role": string(role)})
	return b
}

// encodeConsentRequest builds the consent request the owner's prompt shows.
func encodeConsentRequest(req bark.ConsentRequestPayload) []byte {
	b, _ := json.Marshal(struct {
		Type         string                     `json:"type"`
		Request      bark.ConsentRequestPayload `json:"request"`
		NameVerified bool                       `json:"name_verified"`
	}{Type: ConsentRequestType, Request: req})
	return b
}
