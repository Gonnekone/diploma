package main

import (
	"encoding/json"
	"net/http"
	"v/practice/livekit"
)

var lk = livekit.NewLiveKitServer(
	"http://localhost:7880",
	"devkey",
	"secret",
)

func main() {
	http.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		room := r.URL.Query().Get("room")
		identity := r.URL.Query().Get("identity")

		token, err := lk.GenerateToken(room, identity, identity)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"token": token,
		})
	})

	http.ListenAndServe(":3000", nil)
}
