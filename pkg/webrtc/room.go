package webrtc

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"v/internal/model"

	"github.com/gofiber/websocket/v2"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v3"
)

// вместо 3 секунд — 200 мс
const pcmBufferSize = model.SampleRate / 5

func RoomConn(c *websocket.Conn, p *Peers) {
	var config webrtc.Configuration
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		config = turnConfig
	}
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}
	defer peerConnection.Close()

	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	newPeer := PeerConnectionState{
		PeerConnection: peerConnection,
		Websocket: &ThreadSafeWriter{
			Conn:  c,
			Mutex: sync.Mutex{},
		}}

	p.ListLock.Lock()
	p.Connections = append(p.Connections, newPeer)
	p.ListLock.Unlock()

	log.Println(p.Connections)

	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}
		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}
		if writeErr := newPeer.Websocket.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	peerConnection.OnConnectionStateChange(func(pp webrtc.PeerConnectionState) {
		switch pp {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			log.Printf("Peer connection closed/failed")
			p.ListLock.Lock()
			// Удаляем именно это соединение
			for i := range p.Connections {
				if p.Connections[i].PeerConnection == peerConnection {
					p.Connections = append(p.Connections[:i], p.Connections[i+1:]...)
					break
				}
			}
			p.ListLock.Unlock()

			p.SignalPeerConnections()
		}
	})

	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		trackLocal := p.AddTrack(t, peerConnection)
		if trackLocal == nil {
			return
		}
		defer p.RemoveTrack(trackLocal)

		// аудиотрек обрабатываем через модель шумоподавления
		//if t.Kind() == webrtc.RTPCodecTypeAudio {
		//	go passthroughAudio(t, trackLocal)
		//	return
		//}

		// видеотрек — просто пересылаем без изменений
		buf := make([]byte, 1500)
		for {
			i, _, err := t.Read(buf)
			if err != nil {
				return
			}
			if _, err = trackLocal.Write(buf[:i]); err != nil {
				return
			}
		}
	})

	p.SignalPeerConnections()
	message := &websocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}
			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}
			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
			//case "noise_toggle":
			//	newPeer.NoiseEnabled.Store(message.Data == "true")
			//	log.Printf("noise suppression: %s", message.Data)
		}
	}
}

// processAudioTrack читает RTP аудио, применяет шумоподавление и пишет в локальный трек
func processAudioTrack(
	remote *webrtc.TrackRemote,
	local *webrtc.TrackLocalStaticRTP,
	noiseEnabled *atomic.Bool,
) {
	// инициализируем Opus декодер: 48кГц, моно
	dec, err := opus.NewDecoder(model.SampleRate, 1)
	if err != nil {
		log.Printf("opus decoder init: %v — audio passthrough", err)
		passthroughAudio(remote, local)
		return
	}

	// инициализируем Opus энкодер: 48кГц, моно, VoIP режим
	enc, err := opus.NewEncoder(model.SampleRate, 1, opus.AppVoIP)
	if err != nil {
		log.Printf("opus encoder init: %v — audio passthrough", err)
		passthroughAudio(remote, local)
		return
	}

	// загружаем модель шумоподавления
	suppressor, err := model.NewSuppressor("noisecanceletionmodel/modeldata/noise_suppressor.onnx")
	if err != nil {
		log.Printf("noise suppressor init: %v — audio passthrough", err)
		passthroughAudio(remote, local)
		return
	}
	defer suppressor.Close()

	log.Println("noise suppressor active")

	// PCM буфер для накопления 3 секунд аудио
	pcmBuffer := make([]float32, 0, pcmBufferSize)
	// декодированный фрейм: Opus обычно 20мс = 960 семплов при 48кГц
	pcmFrame := make([]float32, 960)
	rtpBuf := make([]byte, 1500)

	for {
		n, _, err := remote.Read(rtpBuf)
		if err != nil {
			return
		}

		// декодируем Opus → PCM
		samplesDecoded, err := dec.DecodeFloat32(rtpBuf[:n], pcmFrame)
		if err != nil {
			log.Printf("opus decode: %v", err)
			continue
		}

		pcmBuffer = append(pcmBuffer, pcmFrame[:samplesDecoded]...)

		// когда набрали 3 секунды — применяем шумоподавление
		if len(pcmBuffer) < pcmBufferSize {
			continue
		}

		chunk := make([]float32, pcmBufferSize)
		copy(chunk, pcmBuffer[:pcmBufferSize])
		pcmBuffer = pcmBuffer[pcmBufferSize:]

		var denoised []float32
		if noiseEnabled.Load() {
			denoised, err = suppressor.Denoise(chunk)
			if err != nil {
				log.Printf("denoise: %v", err)
				denoised = chunk // при ошибке отдаём исходный
			}
		} else {
			denoised = chunk // просто пропускаем без обработки
		}

		// кодируем PCM обратно в Opus и пишем в локальный трек
		encBuf := make([]byte, 1500)
		frameSize := 960 // 20мс при 48кГц
		for i := 0; i+frameSize <= len(denoised); i += frameSize {
			encoded, err := enc.EncodeFloat32(denoised[i:i+frameSize], encBuf)
			if err != nil {
				log.Printf("opus encode: %v", err)
				continue
			}
			if _, err = local.Write(encBuf[:encoded]); err != nil {
				return
			}
		}
	}
}

// passthroughAudio — запасной вариант: просто пересылаем аудио без обработки
func passthroughAudio(remote *webrtc.TrackRemote, local *webrtc.TrackLocalStaticRTP) {
	buf := make([]byte, 1500)
	for {
		i, _, err := remote.Read(buf)
		if err != nil {
			return
		}
		if _, err = local.Write(buf[:i]); err != nil {
			return
		}
	}
}
