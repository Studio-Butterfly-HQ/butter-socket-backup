// WsHandler handles WebSocket connections
package handler

import (
	"butter-time/internal/hub"
	"butter-time/internal/model"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func HumanAgentHandler(h *hub.Hub, w http.ResponseWriter, r *http.Request) {
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

	userToken := r.URL.Query().Get("token")
	if userToken == "" {
		log.Println("Missing token parameter")
		closeConn(websocket.ClosePolicyViolation, "missing token")
		return
	}
	log.Printf("Employee connection attempt with token: %s...", userToken[:min(10, len(userToken))])

	// request to auth service
	req, err := http.NewRequest(
		http.MethodGet,
		//"https://api.studiobutterfly.io/users/socket/essential",
		"http://localhost:5599/users/socket/essential",
		bytes.NewBuffer([]byte(`{}`)),
	)
	if err != nil {
		log.Println("Request creation failed:", err)
		closeConn(websocket.CloseInternalServerErr, "internal error")
		return
	}

	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Println("Auth API error:", err)
		closeConn(websocket.CloseTryAgainLater, "auth service unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Auth failed with status: %d", resp.StatusCode)
		closeConn(websocket.ClosePolicyViolation, "unauthorized")
		return
	}

	var result model.EssentialResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Println("Decode error:", err)
		closeConn(websocket.CloseInternalServerErr, "invalid auth response")
		return
	}

	// Validate response User
	if result.User.UserID == "" {
		log.Println("No user ID in response")
		closeConn(websocket.ClosePolicyViolation, "invalid user User")
		return
	}

	if len(result.User.Departments) == 0 {
		log.Println("No departments found for user")
		closeConn(websocket.ClosePolicyViolation, "no departments assigned")
		return
	}

	log.Printf("Employee authenticated: %s, Company: %s, Departments: %d",
		result.User.UserID, result.User.CompanyID, len(result.User.Departments))

	// Log departments
	for _, dept := range result.User.Departments {
		log.Printf("   - Department: %s (ID: %s)", dept.DepartmentName, dept.DepartmentID)
	}
	// Create a new client with parameters from query string

	humanAgent := &model.HumanAgentPass{
		Id:          result.User.UserID,
		CompanyId:   result.User.CompanyID,
		Departments: result.User.Departments,
	}
	wsClient := &hub.Client{
		Type:           "Human-Agent",
		Hub:            h,
		Conn:           conn,
		HumanAgentPass: humanAgent,
		Send:           make(chan []byte, 256),
		SosFlag:        true,
		FlagRevealed:   true,
	}
	h.RegisterClient(wsClient)

	// Start goroutines for reading and writing
	go writePump(wsClient)
	go readPump(wsClient)
}
