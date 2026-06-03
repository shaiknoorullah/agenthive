// Package protocols defines libp2p stream protocol IDs, the GossipSub topic,
// and the JSON message types exchanged between agenthive peers.
//
// Stream framing: all messages are JSON-serialised with a 4-byte big-endian
// length prefix. The maximum frame body is 16 MiB; larger frames are rejected
// by ReadFramed.
package protocols

// Stream protocol IDs and the GossipSub topic name.
const (
	ProtoActionRequest  = "/agenthive/action/request/1"
	ProtoActionResponse = "/agenthive/action/response/1"
	ProtoNotification   = "/agenthive/notification/1"
	ProtoPeerAnnounce   = "/agenthive/peer/announce/1"

	TopicState = "/agenthive/state/v1"
)
