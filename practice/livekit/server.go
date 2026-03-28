package livekit

import (
	"context"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type Server struct {
	host       string
	apiKey     string
	apiSecret  string
	roomClient *lksdk.RoomServiceClient
}

func NewLiveKitServer(host, apiKey, apiSecret string) *Server {
	return &Server{
		host:       host,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		roomClient: lksdk.NewRoomServiceClient(host, apiKey, apiSecret),
	}
}

func (s *Server) CreateRoom(roomName string, maxParticipants int32) (*livekit.Room, error) {
	room, err := s.roomClient.CreateRoom(
		context.Background(),
		&livekit.CreateRoomRequest{
			Name:            roomName,
			EmptyTimeout:    300,
			MaxParticipants: uint32(maxParticipants),
		},
	)
	if err != nil {
		return nil, err
	}
	return room, nil
}

func (s *Server) GenerateToken(roomName, identity, participantName string) (string, error) {
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}

	at.SetVideoGrant(grant).
		SetIdentity(identity).
		SetName(participantName).
		SetValidFor(24 * time.Hour)

	return at.ToJWT()
}

func (s *Server) SimulatePublishTrack(roomName, identity string) error {
	_, err := s.GenerateToken(roomName, identity, "bench-user")
	return err
}
