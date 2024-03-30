package main

import (
	"github.com/rileys-trash-can/rs8"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: log <serial>")
	}

	port := os.Args[1]
	conn, err := rs8.Open(port)
	if err != nil {
		log.Fatalf("Failed to connect: %s", err)
	}

	conn.InitBlynk()

	ch := conn.ReadCh()

	for {
		e := <-ch

		switch event := e.(type) {
		case *rs8.EventButton:
			log.Printf("%T %#v", event, event)

		case *rs8.EventSlider:
			log.Printf("%X %3d", event.Type, event.Value)
		}
	}
}
