package main

import (
	"github.com/rileys-trash-can/newtecrs82obs"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: log <serial>")
	}

	port := os.Args[1]
	conn, err := nt8.Open(port)
	if err != nil {
		log.Fatalf("Failed to connect: %s", err)
	}

	conn.InitBlynk()

	ch := conn.ReadCh()

	for {
		e := <-ch

		switch event := e.(type) {
		case *nt8.EventButton:
			log.Printf("%T %#v", event, event)

		case *nt8.EventSlider:
			log.Printf("%X %3d", event.Type, event.Value)
		}
	}
}
