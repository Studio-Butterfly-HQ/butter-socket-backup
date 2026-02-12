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
	exist_customer := h.GetCustomerById(customerId)
	sosFlag := false
	flagRevealed := false
	humanAgentPass := &model.HumanAgentPass{}
	if len(exist_customer) > 0 {
		sosFlag = exist_customer[0].SosFlag
		flagRevealed = exist_customer[0].FlagRevealed
		humanAgentPass = exist_customer[0].HumanAgentPass
	}
	client := &hub.Client{
		Type: "Customer",
		Hub:  h,
		Conn: conn,
		Send: make(chan []byte, 256),
		CustomerPass: &model.CustomerPass{
			Id:        customerId,
			CompanyId: "1234567890",
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
