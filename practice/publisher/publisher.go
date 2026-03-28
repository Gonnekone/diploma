package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	host := "http://localhost:7880"
	apiKey := "devkey"
	apiSecret := "secret"

	// Создаём RoomServiceClient
	roomClient := lksdk.NewRoomServiceClient(host, apiKey, apiSecret)

	// Создаём комнату
	roomName := "bench-room"
	_, err := roomClient.CreateRoom(context.Background(), &livekit.CreateRoomRequest{
		Name:            roomName,
		EmptyTimeout:    0,
		MaxParticipants: 1000,
	})
	if err != nil {
		log.Printf("Room may already exist: %v", err)
	} else {
		log.Printf("Room %s created", roomName)
	}

	participants := 10 // сколько "виртуальных участников"
	for i := 0; i < participants; i++ {
		go func(i int) {
			identity := fmt.Sprintf("user-%d", i)
			token := generateToken(apiKey, apiSecret, roomName, identity)

			// В Go нет полноценного headless WebRTC, но мы можем эмулировать publish
			log.Printf("[%s] Simulating publish track...", identity)

			// Используем LiveKit server SDK метод для генерации
			// токена и публикации без реального медиапотока
			_ = token
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			log.Printf("[%s] Track published (simulated)", identity)
		}(i)
	}

	// Ждём пока все "участники" отработают
	time.Sleep(5 * time.Second)
}

// Генерация токена для участника
func generateToken(apiKey, apiSecret, roomName, identity string) string {
	at := auth.NewAccessToken(apiKey, apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	at.SetVideoGrant(grant).SetIdentity(identity).SetName(identity).SetValidFor(time.Hour)
	jwt, err := at.ToJWT()
	if err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}
	return jwt
}
