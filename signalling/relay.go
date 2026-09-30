// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import "encoding/json"

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
)

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
