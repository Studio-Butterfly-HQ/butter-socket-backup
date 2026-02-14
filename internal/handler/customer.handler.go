package handler

// import (
// 	"butter-time/internal/hub"
// 	"butter-time/internal/model"
// 	"log"
// 	"net/http"
// )

// // WsHandler handles WebSocket connections
// func CustomerHandler(h *hub.Hub, w http.ResponseWriter, r *http.Request) {
// 	conn, err := upgrader.Upgrade(w, r, nil)
// 	if err != nil {
// 		log.Println("Error while upgrading connection:", err)
// 		return
// 	}

// 	// Extract query parameters
// 	queryParams := r.URL.Query()
// 	customerId := queryParams.Get("customer_id")

// 	// Create a new client with parameters from query string

// 	//checking exits.....<<<<()())))
// 	sosFlag := false
// 	flagRevealed := false
// 	humanAgentPass := &model.HumanAgentPass{}
// 	if h.AcceptedCustomers[customerId] != nil {
// 		flagRevealed = true
// 		sosFlag = true
// 		humanAgentPass = h.AcceptedCustomers[customerId]
// 	}
// 	client := &hub.Client{
// 		Type: "Customer",
// 		Hub:  h,
// 		Conn: conn,
// 		Send: make(chan []byte, 256),
// 		CustomerPass: &model.CustomerPass{
// 			Id:        customerId,
// 			CompanyId: "ac0e82ac-1b29-4c91-b553-9d5568ec5faf",
// 		},
// 		SosFlag:        sosFlag,
// 		FlagRevealed:   flagRevealed,
// 		HumanAgentPass: humanAgentPass,
// 	}

// 	log.Printf("New WebSocket connection - Customer ID: %s ", customerId)

// 	// Register the client
// 	client.Hub.RegisterClient(client)

// 	// Start goroutines for reading and writing
// 	go writePump(client)
// 	go readPump(client)
// }
import (
	"butter-time/internal/hub"
	"butter-time/internal/model"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func CustomerHandler(h *hub.Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error while upgrading connection:", err)
		return
	}

	closeConn := func(code int, msg string) {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, msg),
			time.Now().Add(time.Second),
		)
		conn.Close()
	}

	customerToken := r.URL.Query().Get("token")
	if customerToken == "" {
		log.Println("Missing token parameter")
		closeConn(websocket.ClosePolicyViolation, "missing token")
		return
	}

	log.Printf("Customer connection attempt with token: %s...", customerToken[:min(10, len(customerToken))])

	// Call customer profile API
	req, err := http.NewRequest(
		http.MethodGet,
		"https://api.studiobutterfly.io/customer/profile",
		nil,
	)
	if err != nil {
		log.Println("Request creation failed:", err)
		closeConn(websocket.CloseInternalServerErr, "internal error")
		return
	}

	req.Header.Set("Authorization", "Bearer "+customerToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Println("Customer auth API error:", err)
		closeConn(websocket.CloseTryAgainLater, "auth service unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Customer auth failed with status: %d", resp.StatusCode)
		closeConn(websocket.ClosePolicyViolation, "unauthorized")
		return
	}

	var result model.CustomerProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Println("Decode error:", err)
		closeConn(websocket.CloseInternalServerErr, "invalid auth response")
		return
	}

	// Validate response
	if !result.Success || result.Data.ID == "" {
		log.Println("Invalid customer profile response")
		closeConn(websocket.ClosePolicyViolation, "invalid customer data")
		return
	}

	log.Printf("Customer authenticated: %s, Company: %s",
		result.Data.ID,
		result.Data.CompanyID,
	)

	fmt.Printf("Incoming ID: '%s'\n", result.Data.ID)

	//checking exits.....<<<<()())))
	fmt.Println("accepted customers:", h.AcceptedCustomers)
	fmt.Println("accepted customers by id:", h.AcceptedCustomers[result.Data.ID])

	sosFlag := false
	flagRevealed := false
	humanAgentPass := &model.HumanAgentPass{}
	if h.AcceptedCustomers[result.Data.ID] != nil {
		flagRevealed = true
		sosFlag = true
		humanAgentPass = h.AcceptedCustomers[result.Data.ID]
	}
	fmt.Println("Humand agent -> assigned: ", humanAgentPass)
	if h.GetCustomerById(result.Data.ID) != nil {
		sosFlag = h.GetCustomerById(result.Data.ID)[0].SosFlag
	}
	wsClient := &hub.Client{
		Type: "Customer",
		Hub:  h,
		Conn: conn,
		Send: make(chan []byte, 256),
		CustomerPass: &model.CustomerPass{
			Id:        result.Data.ID,
			CompanyId: result.Data.CompanyID,
		},
		SosFlag:        sosFlag,
		FlagRevealed:   flagRevealed,
		HumanAgentPass: humanAgentPass,
	}
	fmt.Println("after ws client creation: ", wsClient.SosFlag, " ", wsClient.FlagRevealed, " ", *wsClient.HumanAgentPass)

	h.RegisterClient(wsClient)

	go writePump(wsClient)
	go readPump(wsClient)
}
