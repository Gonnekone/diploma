package main

import (
	"log"

	"v/internal/server"
)

var Version = "dev"

func main() {
	if err := server.Run(Version); err != nil {
		log.Fatalln(err.Error())
	}
}
