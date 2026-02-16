package main

import (
	"butter-time/internal/handler"
	"butter-time/internal/hub"
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("Starting WebSocket Server...")

	//Create and start the hub
	h := hub.NewHub()
	go h.Run()

	// Setup routes
	//ws://localhost/customer
	http.HandleFunc("/customer", func(w http.ResponseWriter, r *http.Request) {
		handler.CustomerHandler(h, w, r)
	})
	//ws://localhost/customer

	http.HandleFunc("/human-agent", func(w http.ResponseWriter, r *http.Request) {
		handler.HumanAgentHandler(h, w, r)
	})
	//
	// Start server
	addr := "0.0.0.0:4646"
	fmt.Printf("Server listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe error: ", err)
	}
}
