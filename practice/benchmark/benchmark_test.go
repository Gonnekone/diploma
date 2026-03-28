package benchmark

import (
	"testing"
	"v/practice/livekit"
	"v/practice/pion"

	"github.com/pion/webrtc/v3"
)

func peerID(i int) string {
	return "peer-" + string(rune('0'+i%10)) + string(rune('0'+(i/10)%10))
}

func BenchmarkPion_JoinParticipants(b *testing.B) {
	server := pion.NewPionSFUServer()
	server.CreateRoom("bench-room")

	dummyOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP: `v=0
o=- 1234567890 2 IN IP4 127.0.0.1
s=-
t=0 0
a=ice-ufrag:testufrag
a=ice-pwd:testpasswordtestpasswordtest
a=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF
`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peerID := peerID(i)
		_, err := server.AddPeerToRoom("bench-room", peerID, dummyOffer)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLiveKit_JoinParticipants(b *testing.B) {
	lk := livekit.NewLiveKitServer("https://my-livekit-host.com", "API_KEY", "API_SECRET")
	lk.CreateRoom("bench-room-lk", 500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identity := peerID(i)
		_, err := lk.GenerateToken("bench-room-lk", identity, "bench-user")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPion_PublishTrack(b *testing.B) {
	server := pion.NewPionSFUServer()
	server.CreateRoom("bench-room-track")

	dummyOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  "v=0\r\no=- 1234567890 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}

	for i := 0; i < 50; i++ {
		server.AddPeerToRoom("bench-room-track", peerID(i), dummyOffer)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peerID := peerID(100 + i)
		server.AddPeerToRoom("bench-room-track", peerID, dummyOffer)
	}
}

func BenchmarkLiveKit_PublishTrack(b *testing.B) {
	lk := livekit.NewLiveKitServer("https://your-livekit-host.com", "API_KEY", "API_SECRET")
	lk.CreateRoom("bench-room-track-lk", 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		identity := peerID(i)
		if err := lk.SimulatePublishTrack("bench-room-track-lk", identity); err != nil {
			b.Fatal(err)
		}
	}
}

//func BenchmarkPion_JoinAndPublish(b *testing.B) {
//	server := pion.NewPionSFUServer()
//	roomID := "bench-room-pion"
//	server.CreateRoom(roomID)
//
//	dummyOffer := webrtc.SessionDescription{
//		Type: webrtc.SDPTypeOffer,
//		SDP: `v=0
//o=- 1234567890 2 IN IP4 127.0.0.1
//s=-
//t=0 0
//a=ice-ufrag:testufrag
//a=ice-pwd:testpasswordtestpasswordtest
//a=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF
//`,
//	}
//
//	b.ResetTimer()
//	for i := 0; i < b.N; i++ {
//		var wg sync.WaitGroup
//		wg.Add(1) // Для одной горутины на итерацию; можно увеличить для симуляции нескольких пиров
//
//		go func(i int) {
//			defer wg.Done()
//
//			peerID := peerID(i)
//
//			// Подключение (join)
//			_, err := server.AddPeerToRoom(roomID, peerID, dummyOffer)
//			if err != nil {
//				b.Fatal(err)
//			}
//
//			// Симуляция публикации трека
//			room, _ := server.GetRoom(roomID)
//			trackLocal, err := webrtc.NewTrackLocalStaticRTP(
//				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
//				"video", "pion-bench",
//			)
//			if err != nil {
//				b.Fatal(err)
//			}
//
//			room.PeersLock.Lock()
//			room.TrackLocals[peerID] = append(room.TrackLocals[peerID], trackLocal)
//			room.PeersLock.Unlock()
//
//			// Симуляция отправки 50 RTP-пакетов (без sleep, просто цикл)
//			for j := 0; j < 50; j++ {
//				pkt := &rtp.Packet{
//					Header: rtp.Header{
//						Version:        2,
//						SSRC:           uint32(i),
//						SequenceNumber: uint16(j),
//					},
//					Payload: []byte{0x00, 0x01, 0x02}, // Dummy payload
//				}
//				if err := trackLocal.WriteRTP(pkt); err != nil {
//					b.Fatal(err)
//				}
//			}
//
//			// Симуляция ретрансляции (forwardTrack)
//			server.ForwardTrack(room, peerID, trackLocal)
//		}(i)
//
//		wg.Wait() // Ждём завершения всех операций в итерации
//	}
//}
//
//func BenchmarkLiveKit_JoinAndPublish(b *testing.B) {
//	lkHost := "http://localhost:7880" // Или ваш реальный хост
//	apiKey := "devkey"
//	apiSecret := "secret"
//	lk := livekit.NewLiveKitServer(lkHost, apiKey, apiSecret)
//	roomName := "bench-room-livekit"
//	_, _ = lk.CreateRoom(roomName, 1000) // Если нужно; иначе комната создаётся автоматически
//
//	b.ResetTimer()
//	for i := 0; i < b.N; i++ {
//		var wg sync.WaitGroup
//		wg.Add(1) // Для одной горутины; можно увеличить
//
//		go func(i int) {
//			defer wg.Done()
//
//			identity := peerID(i)
//
//			// Подключение (join via token)
//			token, err := lk.GenerateToken(roomName, identity, identity)
//			if err != nil {
//				b.Fatal(err)
//			}
//			_ = token // Симуляция использования токена для join (в реальности — через client SDK)
//
//			// Симуляция создания PeerConnection и трека (как в вашем коде)
//			m := &webrtc.MediaEngine{}
//			if err := m.RegisterDefaultCodecs(); err != nil {
//				b.Fatal(err)
//			}
//			api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
//			pc, err := api.NewPeerConnection(webrtc.Configuration{})
//			if err != nil {
//				b.Fatal(err)
//			}
//
//			track, err := webrtc.NewTrackLocalStaticRTP(
//				webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
//				"video", "livekit",
//			)
//			if err != nil {
//				b.Fatal(err)
//			}
//			_, err = pc.AddTrack(track)
//			if err != nil {
//				b.Fatal(err)
//			}
//
//			// Симуляция отправки 50 RTP-пакетов (без sleep)
//			for j := 0; j < 50; j++ {
//				err := track.WriteRTP(&rtp.Packet{
//					Header:  rtp.Header{Version: 2, SSRC: uint32(i), SequenceNumber: uint16(j)},
//					Payload: []byte{0x00, 0x01, 0x02},
//				})
//				if err != nil {
//					b.Fatal(err)
//				}
//			}
//
//			pc.Close()
//		}(i)
//
//		wg.Wait()
//	}
//}
