package main

import (
	"log"

	"v/internal/server"
)

var Version = "dev" // перезаписывается при сборке через -ldflags

func main() {
	if err := server.Run(Version); err != nil {
		log.Fatalln(err.Error())
	}
}
