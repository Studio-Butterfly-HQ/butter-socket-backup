package handler

import (
	"butter-time/internal/constructor"
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
	switch wsMsg.Type {
	case "transfer_chat":
		if client.Type != "Customer" {
			sendError(client, "you're not allowed for this request")
			return
		}
		handleChatTransferToHumanAgent(client, wsMsg.Payload)
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

// trigger name: transfer_chat (currently working on this...)
/*
  -> transfer chat is a trigger that is responsible for sending a communication
   request of a customer to all human agents. if no one is available to receive the request that it will
   be qued. if one person comes he will get it instantly. unless it gets accepted by
   any agent the request will remain in the queue and everytime new agent logs in he
   will receive the queue....:.....
*/
func handleChatTransferToHumanAgent(client *hub.Client, payload any) {
	// 1. cheking the sos flag -> to processed // else duplicate request (done...)
	if !client.SosFlag {
		client.SosFlag = true
		//-> step1-> creating conversation payload
		conversation, err := constructor.ConversationPayloadConstructor(payload, false)
		connList := client.Hub.GetHumanAgents() //<- all active agents (done without company isolation)
		if len(connList) == 0 {
			// no one is available : notify the customer (done...)
			// then -> add to the companies pending queue...
			unavilableMsgPayload := model.MsgInOut{
				SenderType: "System",
				SenderId:   "butter-chat",
				Content:    "added to queue list",
			}
			//append to the pending queue of the company
			client.Hub.AddToPendingChat(client.CustomerPass.CompanyId, conversation)
			sendMessage(client, "connection_event", unavilableMsgPayload)
		} else {
			//everyting alright
			//-> step2-> sending the payload as byte in SendMessage()
			//step:1
			if err != nil {
				fmt.Println("error decoding to byte: conversation payload")
				sendMessage(client, "connection_event", "server error")
			}
			//step:2
			//boradcasting the conversation -|-------->>>>>[Human Agents]
			for _, conn := range connList {
				sendMessage(conn, "transfer_chat", conversation)
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
// -> updates the customer client
// -> add more data to the conversation payload
// -> add to the human agents inbox queue
// -> broadcast accept event to user devices
// -> broadcast conversation to the human agent devices inbox
// -> remove from pending list for all agents
func handleHumanAcceptTheChat(client *hub.Client, payload any) {
	conversation, err := constructor.ConversationPayloadConstructor(payload, true)
	if err != nil {
		fmt.Println("error decoding to byte: conversation payload")
		sendMessage(client, "connection_event", "server error")
	}
	//update the assigned person to self....
	conversation.AssignedTo = &model.AssignedTo{
		Id: client.HumanAgentPass.Id,
		//Name: "Dummy Name", //need to update api response
		//ProfileUri: "Dummy ",
	}
	conversation.Status = "on going"
	//mark the customer accepted and connected to human:
	///->
	client.Hub.MarkCustomerAccepted(
		conversation.CustomerPayload.Id,
		client.HumanAgentPass,
	)
	//push to the queue for this agent list of map:
	client.Hub.AddToActiveChat(
		client.HumanAgentPass.Id,
		conversation,
	)
	//todo: remove from pending list of all devices of this company - (*department*):
	client.Hub.RemoveFromPending(
		client.HumanAgentPass.CompanyId,
		conversation.CustomerPayload.Id,
	)
	//----------------------------------------
	//--->> broadcast the inbox to all devices of the Human Agent
	//human agent devics:
	agentDevices := client.Hub.GetHumanAgentById(client.HumanAgentPass.Id)
	//send to inbox:
	for _, agent := range agentDevices {
		sendMessage(agent, "accept_chat", conversation)
	}
	//send the accept flag to the customer....
	customer := client.Hub.GetCustomerById(conversation.CustomerPayload.Id)
	fmt.Println(customer)
	if len(customer) == 0 {
		msg := model.WSMessage{
			Type:    "connection_event",
			Payload: "connected to human",
		}
		client.Hub.AddEventToCustomerEventQueue(conversation.CustomerPayload.Id, msg)
		sendMessage(client, "connection_event", "customer offline...")
		return
	}
	if len(customer) != 0 && customer[0].FlagRevealed {
		sendMessage(client, "connection_event", "already connected")
		return
	}
	fmt.Println(conversation)
	if conversation.CustomerPayload.Id == "" {
		sendError(client, "invalid payload")
		return
	}
	msg := model.WSMessage{
		Type:    "connection_event",
		Payload: "connected to human",
	}
	client.Hub.AddEventToCustomerEventQueue(conversation.CustomerPayload.Id, msg)
	for _, v := range customer {
		v.FlagRevealed = true
		v.HumanAgentPass = &model.HumanAgentPass{
			Id: client.HumanAgentPass.Id,
			// CompanyId:   client.HumanAgentPass.CompanyId,
			// Departments: client.HumanAgentPass.Departments,
		}
		sendMessage(v, "connection_event", "connection request accepted")
	}
	sendMessage(client, "connection_event", "connection stablished")
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
			//meessage adding to customer queue:
			client.Hub.AddMessageToCustomerQueue(data.ReceiverId, payload)
			//client.Hub.CustomerMessageQueue[data.ReceiverId] = append(client.Hub.CustomerMessageQueue[data.ReceiverId], payload)

			//message adding to human agent queue:
			client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, payload)
			//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], payload)

			fmt.Println(len(client.Hub.CustomerMessageQueue))
			fmt.Println(client.Hub.CustomerMessageQueue[data.ReceiverId])
			sendMessage(client, "connection_event", "customer offline, message added to customer message queue")
			return
		}
		client.Hub.AddMessageToCustomerQueue(data.ReceiverId, payload)
		client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, payload)

		// client.Hub.CustomerMessageQueue[data.ReceiverId] = append(client.Hub.CustomerMessageQueue[data.ReceiverId], payload)
		// client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], payload)
		for _, v := range customer {
			sendMessage(v, "message", payload)
		}
		//broadcast to all agent devices//
		for _, v := range client.Hub.GetHumanAgentById(client.HumanAgentPass.Id) {
			sendMessage(v, "message", payload)
		}
		//............................................//
	} else if client.Type == "Customer" {
		humanAgent := client.Hub.GetHumanAgentById(client.HumanAgentPass.Id)
		if len(humanAgent) == 0 {
			client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, payload)
			client.Hub.AddMessageToCustomerQueue(client.CustomerPass.Id, payload)
			//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], payload)

			sendMessage(client, "connection_event", "agent offline, message added to agent queue")
			return
		}
		client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, payload)
		client.Hub.AddMessageToCustomerQueue(client.CustomerPass.Id, payload)

		//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], payload)
		//client.Hub.CustomerMessageQueue[client.CustomerPass.Id] = append(client.Hub.CustomerMessageQueue[client.CustomerPass.Id], payload)
		for _, v := range humanAgent {
			sendMessage(v, "message", payload)
		}
		for _, v := range client.Hub.GetCustomerById(client.CustomerPass.Id) {
			sendMessage(v, "message", payload)
		}
	}
}
