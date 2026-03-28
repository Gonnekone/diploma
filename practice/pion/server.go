package pion

import (
	"errors"
	"sync"

	"github.com/pion/webrtc/v3"
)

var ErrRoomNotFound = errors.New("room not found")

type SFUServer struct {
	rooms     map[string]*Room
	roomsLock sync.RWMutex
	api       *webrtc.API
}

type Room struct {
	id          string
	peers       map[string]*Peer
	PeersLock   sync.RWMutex
	TrackLocals map[string][]*webrtc.TrackLocalStaticRTP // peerID -> tracks
}

type Peer struct {
	id string
	pc *webrtc.PeerConnection
}

func NewPionSFUServer() *SFUServer {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(settingEngine))

	return &SFUServer{
		rooms: make(map[string]*Room),
		api:   api,
	}
}

func (s *SFUServer) CreateRoom(roomID string) {
	s.roomsLock.Lock()
	defer s.roomsLock.Unlock()

	if _, exists := s.rooms[roomID]; exists {
		return
	}

	s.rooms[roomID] = &Room{
		id:          roomID,
		peers:       make(map[string]*Peer),
		TrackLocals: make(map[string][]*webrtc.TrackLocalStaticRTP),
	}
}

func (s *SFUServer) GetRoom(roomID string) (*Room, bool) {
	s.roomsLock.RLock()
	defer s.roomsLock.RUnlock()
	r, ok := s.rooms[roomID]
	return r, ok
}

func (s *SFUServer) AddPeerToRoom(roomID, peerID string, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	room, exists := s.GetRoom(roomID)
	if !exists {
		return webrtc.SessionDescription{}, ErrRoomNotFound
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := s.api.NewPeerConnection(config)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	pc.OnTrack(func(trackRemote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		trackLocal, err := webrtc.NewTrackLocalStaticRTP(trackRemote.Codec().RTPCodecCapability, trackRemote.ID(), trackRemote.StreamID())
		if err != nil {
			return
		}

		room.PeersLock.Lock()
		room.TrackLocals[peerID] = append(room.TrackLocals[peerID], trackLocal)
		room.PeersLock.Unlock()

		s.ForwardTrack(room, peerID, trackLocal)

		go func() {
			for {
				pkt, _, err := trackRemote.ReadRTP()
				if err != nil {
					return
				}
				trackLocal.WriteRTP(pkt)
			}
		}()
	})

	room.PeersLock.RLock()
	for otherID, tracks := range room.TrackLocals {
		if otherID == peerID {
			continue
		}
		for _, track := range tracks {
			pc.AddTrack(track)
		}
	}
	room.PeersLock.RUnlock()

	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, err
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return webrtc.SessionDescription{}, err
	}

	room.PeersLock.Lock()
	room.peers[peerID] = &Peer{id: peerID, pc: pc}
	room.PeersLock.Unlock()

	return answer, nil
}

func (s *SFUServer) ForwardTrack(room *Room, senderID string, track *webrtc.TrackLocalStaticRTP) {
	room.PeersLock.RLock()
	defer room.PeersLock.RUnlock()

	for peerID, peer := range room.peers {
		if peerID == senderID {
			continue
		}
		peer.pc.AddTrack(track)
	}
}
