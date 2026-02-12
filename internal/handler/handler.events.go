package handler

import (
	"butter-time/internal/hub"
	"butter-time/internal/model"
	"encoding/json"
	"fmt"
	"log"
)

func handleIncomingMessage(client *hub.Client, message []byte) {
	var wsMsg model.WSMessage
	if err := json.Unmarshal(message, &wsMsg); err != nil {
		sendError(client, "Invalid WS message format")
		return
	}
	fmt.Println(wsMsg)
	fmt.Println(client.FlagRevealed)
	fmt.Println(client.SosFlag)
	//fmt.Println(&client.HumanAgentPass)
	switch wsMsg.Type {
	case "transfer_chat":
		if client.Type != "Customer" {
			sendError(client, "you're not allowed for this request")
			return
		}
		handleChatTransferToUser(client, wsMsg.Payload)
	case "accept_chat":
		if client.Type != "Human-Agent" {
			sendError(client, "you're not allowed for this request")
			return
		}
		handleHumanAcceptTheChat(client, wsMsg.Payload)
	case "message":
		if client.FlagRevealed == true {
			fmt.Println("Client Type: ", client.Type)
			handleConversationWithHuman(client, wsMsg.Payload)
		} else {
			sendMessage(client, "event", "ai not available")
			//handleChatStreamMessage(client, wsMsg.Payload)
		}
	case "ping":
		sendPong(client)
	default:
		sendError(client, "Unknown message type")
	}
}

// sendMessage sends a message to a specific client
func sendMessage(client *hub.Client, msgType string, payload interface{}) {
	wsMsg := model.WSMessage{
		Type:    msgType,
		Payload: payload,
	}

	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Println("Error marshaling message:", err)
		return
	}

	select {
	case client.Send <- msgBytes:
	default:
		log.Println("Client send channel is full")
	}
}

// sendError sends an error message to the client
func sendError(client *hub.Client, errorMsg string) {
	errorPayload := map[string]string{
		"error": errorMsg,
	}
	sendMessage(client, "error", errorPayload)
}

// sendPong responds to ping messages
func sendPong(client *hub.Client) {
	pongPayload := map[string]string{
		"status": "pong",
	}
	sendMessage(client, "pong", pongPayload)
}

// trigger name: transfer_chat
func handleChatTransferToUser(client *hub.Client, payload any) {
	if !client.SosFlag {
		client.SosFlag = true
		connList := client.Hub.GetHumanAgents()
		if len(connList) == 0 {
			unavilableMsgPayload := model.MsgInOut{
				SenderType: "system",
				SenderId:   "system",
				Content:    "added to queue list",
			}
			client.Hub.MessageQueue = append(client.Hub.MessageQueue, payload)
			sendMessage(client, "connection_event", unavilableMsgPayload)
		} else {
			for _, conn := range connList {
				sendMessage(conn, "transfer_chat", payload)
			}
		}
	} else {
		duplicateMsgPayload := model.MsgInOut{
			SenderType: "system",
			SenderId:   "system",
			Content:    "duplicate request",
		}
		sendMessage(client, "connection_event", duplicateMsgPayload)
	}
}

// trigger name: accept_chat (for users)
func handleHumanAcceptTheChat(client *hub.Client, payload any) {
	//todooooo->>>>>>> check the client is already accepted or not... [on going]
	payloadByte, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return
	}
	var data model.MsgInOut
	json.Unmarshal(payloadByte, &data)
	customer := client.Hub.GetCustomerById(data.ReceiverId)
	if len(customer) != 0 && customer[0].FlagRevealed {
		sendMessage(client, "connection_event", "already connected")
		return
	}
	fmt.Println(data)
	if data.ReceiverId == "" {
		sendError(client, "invalid payload")
		return
	}
	fmt.Println(customer)
	if len(customer) == 0 {
		//maybe i will add this in the queue later.... maybeeeeeeeee ----:) :()--->>>>
		sendMessage(client, "connection_event", "customer gone")
		return
	}
	for _, v := range customer {
		v.FlagRevealed = true
		v.HumanAgentPass = &model.HumanAgentPass{
			Id:          client.HumanAgentPass.Id,
			CompanyId:   client.HumanAgentPass.CompanyId,
			Departments: client.HumanAgentPass.Departments,
		}
		sendMessage(v, "connection_event", "connection request accepted")
	}
}

// trigger name: message
func handleConversationWithHuman(client *hub.Client, payload any) {
	//-> steps:
	//1. construct the payload
	payloadByte, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return
	}
	var data model.MsgInOut
	json.Unmarshal(payloadByte, &data)
	fmt.Println("message Data: ", data)

	if client.Type == "Human-Agent" {
		if data.ReceiverId == "" {
			sendError(client, "invalid payload")
			return
		}
		customer := client.Hub.GetCustomerById(data.ReceiverId)
		fmt.Println("customer: ->", customer)
		if len(customer) != 0 && customer[0].FlagRevealed != true {
			sendMessage(client, "connection_event", "you're not allowed to text unless customer wants")
			return
		}
		if len(customer) == 0 {
			client.Hub.CustomerQueue[data.ReceiverId] = append(client.Hub.CustomerQueue[data.ReceiverId], payload)
			fmt.Println(len(client.Hub.CustomerQueue))
			fmt.Println(client.Hub.CustomerQueue[data.ReceiverId])
			return
		}
		for _, v := range customer {
			sendMessage(v, "message", payload)
		}
		//broadcast to all agent devices//..--<-todo
	} else if client.Type == "Customer" {
		humanAgent := client.Hub.GetHumanAgentById(client.HumanAgentPass.Id)
		if len(humanAgent) == 0 {
			client.Hub.HumanAgentQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentQueue[client.HumanAgentPass.Id], payload)
			sendMessage(client, "connection_event", "agent offline, message added to agent queue")
			return
		}
		for _, v := range humanAgent {
			sendMessage(v, "message", payload)
		}
	}
}
