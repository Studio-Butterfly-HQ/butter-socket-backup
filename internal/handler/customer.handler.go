package handler

import (
	"butter-time/internal/hub"
	"butter-time/internal/model"
	"log"
	"net/http"
)

// WsHandler handles WebSocket connections
func CustomerHandler(h *hub.Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error while upgrading connection:", err)
		return
	}

	// Extract query parameters
	queryParams := r.URL.Query()
	customerId := queryParams.Get("customer_id")

	// Create a new client with parameters from query string

	//checking exits.....<<<<()())))
	sosFlag := false
	flagRevealed := false
	humanAgentPass := &model.HumanAgentPass{}
	if h.AcceptedCustomers[customerId] != nil {
		flagRevealed = true
		sosFlag = true
		humanAgentPass = h.AcceptedCustomers[customerId]
	}
	client := &hub.Client{
		Type: "Customer",
		Hub:  h,
		Conn: conn,
		Send: make(chan []byte, 256),
		CustomerPass: &model.CustomerPass{
			Id:        customerId,
			CompanyId: "ac0e82ac-1b29-4c91-b553-9d5568ec5faf",
		},
		SosFlag:        sosFlag,
		FlagRevealed:   flagRevealed,
		HumanAgentPass: humanAgentPass,
	}

	log.Printf("New WebSocket connection - Customer ID: %s ", customerId)

	// Register the client
	client.Hub.RegisterClient(client)

	// Start goroutines for reading and writing
	go writePump(client)
	go readPump(client)
}
