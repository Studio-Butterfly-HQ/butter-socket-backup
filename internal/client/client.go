package client

// import (
// 	"butter-time/internal/hub"
// 	"butter-time/internal/model"
// 	"context"

// 	"github.com/gorilla/websocket"
// )

// type Client struct {
// 	hub          hub.Hub
// 	Type         string
// 	Conn         *websocket.Conn
// 	Send         chan []byte
// 	Customer     *model.CustomerPass
// 	User         *model.HumanAgentPass
// 	CancelAI     context.CancelFunc
// 	SosFlag      bool // -> true when customer talking to human or need to talk to human
// 	FlagRevealed bool // -> when a human accepts connection
// }
