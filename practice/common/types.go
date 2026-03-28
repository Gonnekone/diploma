package common

import "encoding/json"

type Room struct {
	ID        string
	Clients   map[string]*Client
	Broadcast chan []byte
}

type Client struct {
	ID       string
	RoomID   string
	Send     chan []byte
	Metadata map[string]interface{}
}

type SignalingMessage struct {
	Type    string          `json:"type"`
	From    string          `json:"from,omitempty"`
	To      string          `json:"to,omitempty"`
	RoomID  string          `json:"room_id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
