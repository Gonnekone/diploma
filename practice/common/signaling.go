package common

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type JoinPayload struct {
	DisplayName string            `json:"display_name"`
	Role        string            `json:"role"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type RoomInfo struct {
	ID           string            `json:"id"`
	Participants []ParticipantInfo `json:"participants"`
	CreatedAt    time.Time         `json:"created_at"`
}

type ParticipantInfo struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	Role        string            `json:"role"`
	JoinedAt    time.Time         `json:"joined_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type SignalingServer struct {
	upgrader     websocket.Upgrader
	connections  map[string]*websocket.Conn
	rooms        map[string]*RoomInfo
	participants map[string]*ParticipantInfo // participantID -> ParticipantInfo
	mu           sync.RWMutex
}

func NewSignalingServer() *SignalingServer {
	return &SignalingServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		connections:  make(map[string]*websocket.Conn),
		rooms:        make(map[string]*RoomInfo),
		participants: make(map[string]*ParticipantInfo),
	}
}

func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	peerID := r.URL.Query().Get("peer_id")
	roomID := r.URL.Query().Get("room_id")

	if peerID == "" {
		peerID = fmt.Sprintf("peer-%d", time.Now().UnixNano())
	}

	s.mu.Lock()
	s.connections[peerID] = conn
	s.mu.Unlock()

	log.Printf("New WebSocket connection: %s, room: %s", peerID, roomID)

	conn.WriteJSON(SignalingMessage{
		Type:    "connected",
		From:    "server",
		To:      peerID,
		Payload: json.RawMessage(fmt.Sprintf(`{"peer_id": "%s"}`, peerID)),
	})

	defer func() {
		s.handleLeave(peerID, roomID)
		s.mu.Lock()
		delete(s.connections, peerID)
		delete(s.participants, peerID)
		s.mu.Unlock()
		conn.Close()
		log.Printf("WebSocket closed: %s", peerID)
	}()

	for {
		var msg SignalingMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if msg.From == "" {
			msg.From = peerID
		}

		log.Printf("Received message from %s: type=%s, to=%s", msg.From, msg.Type, msg.To)

		switch msg.Type {
		case "offer", "answer", "candidate":
			s.relayMessage(msg)
		case "join":
			s.handleJoin(peerID, msg)
		case "leave":
			s.handleLeave(peerID, roomID)
		case "ping":
			conn.WriteJSON(SignalingMessage{
				Type: "pong",
				From: "server",
				To:   peerID,
			})
		case "list_participants":
			s.handleListParticipants(peerID, roomID)
		case "room_info":
			s.handleRoomInfo(peerID, roomID)
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (s *SignalingServer) handleJoin(peerID string, msg SignalingMessage) {
	var payload JoinPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("Failed to parse join payload: %v", err)
		return
	}

	roomID := msg.RoomID
	if roomID == "" {
		log.Printf("Room ID is required for join")
		return
	}

	participant := &ParticipantInfo{
		ID:          peerID,
		DisplayName: payload.DisplayName,
		Role:        payload.Role,
		JoinedAt:    time.Now(),
		Metadata:    payload.Metadata,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		room = &RoomInfo{
			ID:           roomID,
			Participants: []ParticipantInfo{},
			CreatedAt:    time.Now(),
		}
		s.rooms[roomID] = room
		log.Printf("Created new room: %s", roomID)
	}

	room.Participants = append(room.Participants, *participant)
	s.participants[peerID] = participant

	if conn, ok := s.connections[peerID]; ok {
		conn.WriteJSON(SignalingMessage{
			Type:   "joined",
			From:   "server",
			To:     peerID,
			RoomID: roomID,
			Payload: json.RawMessage(fmt.Sprintf(
				`{"room_id": "%s", "participant_id": "%s", "display_name": "%s"}`,
				roomID, peerID, payload.DisplayName,
			)),
		})
	}

	s.broadcastToRoom(roomID, peerID, SignalingMessage{
		Type:   "participant_joined",
		From:   "server",
		RoomID: roomID,
		Payload: json.RawMessage(fmt.Sprintf(
			`{"participant_id": "%s", "display_name": "%s", "role": "%s"}`,
			peerID, payload.DisplayName, payload.Role,
		)),
	})

	log.Printf("Participant %s joined room %s as %s", peerID, roomID, payload.Role)
}

func (s *SignalingServer) handleLeave(peerID, roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if room, exists := s.rooms[roomID]; exists {
		for i, participant := range room.Participants {
			if participant.ID == peerID {
				room.Participants = append(room.Participants[:i], room.Participants[i+1:]...)
				log.Printf("Removed participant %s from room %s", peerID, roomID)

				if len(room.Participants) == 0 {
					delete(s.rooms, roomID)
					log.Printf("Room %s deleted (no participants)", roomID)
				}
				break
			}
		}

		s.broadcastToRoom(roomID, peerID, SignalingMessage{
			Type:   "participant_left",
			From:   "server",
			RoomID: roomID,
			Payload: json.RawMessage(fmt.Sprintf(
				`{"participant_id": "%s"}`,
				peerID,
			)),
		})
	}

	delete(s.participants, peerID)
	log.Printf("Participant %s left room %s", peerID, roomID)
}

func (s *SignalingServer) handleListParticipants(peerID, roomID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if room, exists := s.rooms[roomID]; exists {
		participantsJSON, _ := json.Marshal(room.Participants)

		if conn, ok := s.connections[peerID]; ok {
			conn.WriteJSON(SignalingMessage{
				Type:   "participants_list",
				From:   "server",
				To:     peerID,
				RoomID: roomID,
				Payload: json.RawMessage(fmt.Sprintf(
					`{"participants": %s}`,
					string(participantsJSON),
				)),
			})
		}
	}
}

func (s *SignalingServer) handleRoomInfo(peerID, roomID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if room, exists := s.rooms[roomID]; exists {
		roomInfoJSON, _ := json.Marshal(room)

		if conn, ok := s.connections[peerID]; ok {
			conn.WriteJSON(SignalingMessage{
				Type:   "room_info",
				From:   "server",
				To:     peerID,
				RoomID: roomID,
				Payload: json.RawMessage(fmt.Sprintf(
					`{"room": %s}`,
					string(roomInfoJSON),
				)),
			})
		}
	}
}

func (s *SignalingServer) broadcastToRoom(roomID, excludePeerID string, msg SignalingMessage) {
	if room, exists := s.rooms[roomID]; exists {
		for _, participant := range room.Participants {
			if participant.ID != excludePeerID {
				if conn, ok := s.connections[participant.ID]; ok {
					conn.WriteJSON(msg)
				}
			}
		}
	}
}

func (s *SignalingServer) relayMessage(msg SignalingMessage) {
	if msg.To == "" {
		log.Printf("Message has no recipient: %+v", msg)
		return
	}

	s.mu.RLock()
	conn, exists := s.connections[msg.To]
	s.mu.RUnlock()

	if exists {
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("Failed to relay message to %s: %v", msg.To, err)
		} else {
			log.Printf("Relayed message from %s to %s: type=%s",
				msg.From, msg.To, msg.Type)
		}
	} else {
		log.Printf("Recipient not found: %s", msg.To)
	}
}

func (s *SignalingServer) GetRoomStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]int)
	for roomID, room := range s.rooms {
		stats[roomID] = len(room.Participants)
	}
	return stats
}

func (s *SignalingServer) CloseAllConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for peerID, conn := range s.connections {
		conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutdown"),
			time.Now().Add(5*time.Second))
		conn.Close()
		delete(s.connections, peerID)
	}

	s.rooms = make(map[string]*RoomInfo)
	s.participants = make(map[string]*ParticipantInfo)
}
