package handler

import (
	"butter-time/internal/constructor"
	"butter-time/internal/hub"
	"butter-time/internal/model"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

func handleIncomingMessage(client *hub.Client, message []byte) {
	fmt.Println("incomming message to handler: ", string(message))
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
	case "end_chat":
		if client.Type != "Human-Agent" {
			sendError(client, "you're not allowed for this request")
			return
		}
		handleEndtheChat(client, wsMsg.Payload)
	case "message":
		if client.FlagRevealed == true {
			fmt.Println("Client Type: ", client.Type)
			handleConversationWithHuman(client, wsMsg.Payload)
		} else {
			sendMessage(client, "butter_chat", "ai chat for customers coming soon...")
			//handleChatStreamMessage(client, wsMsg.Payload)
		}
	case "butter_chat":
		if client.Type != "Human-Agent" {
			sendMessage(client, "ai_message", "this event is for butter-chat users only...")
			return
		}
		handleAiStream(client, wsMsg.Payload)

	case "ping":
		fmt.Println("pinging...")
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
	fmt.Println("Transfer Chat : -> ", payload)
	if !client.SosFlag {
		client.SosFlag = true
		client.Hub.SosStatus[client.CustomerPass.Id] = true
		//todo : need to mark all active device true....
		//---->>>><<<<<_______>>>><<<<<<<<<<<<OOOOOOOOOO
		//-> step1-> creating conversation payload
		conversation, err := constructor.ConversationPayloadConstructor(payload, false)
		conversation.CustomerPayload.Id = client.CustomerPass.Id
		conversation.CustomerPayload.CompanyId = client.CustomerPass.CompanyId
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
			sendMessage(client, "pending", unavilableMsgPayload)
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
		fmt.Println("before insert ", len(client.Hub.PendingChatQueue[client.CustomerPass.CompanyId]))
		fmt.Println("company id: ", client.CustomerPass.CompanyId)
		client.Hub.AddToPendingChat(client.CustomerPass.CompanyId, conversation)
		fmt.Println("after insert", len(client.Hub.PendingChatQueue[client.CustomerPass.CompanyId]))
		fmt.Println("company id: ", client.CustomerPass.CompanyId)

	} else {
		duplicateMsgPayload := model.MsgInOut{
			SenderType: "system",
			SenderId:   "system",
			Content:    "duplicate request",
		}
		fmt.Println("before insert ", len(client.Hub.PendingChatQueue[client.CustomerPass.CompanyId]))
		sendMessage(client, "pending", duplicateMsgPayload)
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
	AgentPassSeal := client.HumanAgentPass
	AgentPassSeal.ConversationSeal = conversation.Id
	client.Hub.MarkCustomerAccepted(
		conversation.CustomerPayload.Id,
		AgentPassSeal,
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
	//send to inbox list:
	for _, agent := range agentDevices {
		sendMessage(agent, "accept_chat", conversation)
	}
	//send the accept flag to the customer....
	customer := client.Hub.GetCustomerById(conversation.CustomerPayload.Id)
	fmt.Println(customer)
	if len(customer) == 0 {
		msg := model.WSMessage{
			Type:    "accepted",
			Payload: "connected to human",
		}
		client.Hub.AddEventToCustomerEventQueue(conversation.CustomerPayload.Id, msg)
		sendMessage(client, "customer_offline", "customer offline...")
		return
	}
	if len(customer) != 0 && customer[0].FlagRevealed {
		sendMessage(client, "accepted", "already connected")
		return
	}
	fmt.Println(conversation)
	if conversation.CustomerPayload.Id == "" {
		sendError(client, "invalid payload")
		return
	}
	msg := model.WSMessage{
		Type:    "accepted",
		Payload: "connected to human",
	}
	client.Hub.AddEventToCustomerEventQueue(conversation.CustomerPayload.Id, msg)
	for _, v := range customer {
		v.FlagRevealed = true
		v.HumanAgentPass = &model.HumanAgentPass{
			Id: client.HumanAgentPass.Id,
			// CompanyId:   client.HumanAgentPass.CompanyId,
			// Departments: client.HumanAgentPass.Departments,
			ConversationSeal: conversation.Id,
		}
		sendMessage(v, "connection_stablished", "connection request accepted")
	}
	sendMessage(client, "connection_stablished", "connection stablished")
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
		if data.ReceiverId == "" || data.ConversationId == "" {
			sendError(client, "invalid payload")
			return
		}
		customer := client.Hub.GetCustomerById(data.ReceiverId)
		fmt.Println("customer: ->", customer)
		if len(customer) != 0 && customer[0].FlagRevealed != true {
			sendMessage(client, "connection_event", "you're not allowed to text unless customer wants")
			return
		}
		data.SenderId = client.HumanAgentPass.Id
		data.ContentType = "text"
		data.CreatedAt = time.Now().String()
		data.SenderType = "Human-Agent"
		msgPayload := &model.WSMessage{
			Type:    "message",
			Payload: data,
		}
		if len(customer) == 0 {
			//meessage adding to customer queue:
			client.Hub.AddMessageToCustomerQueue(data.ReceiverId, msgPayload)
			//client.Hub.CustomerMessageQueue[data.ReceiverId] = append(client.Hub.CustomerMessageQueue[data.ReceiverId], msgPayload)

			//message adding to human agent queue:
			client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, msgPayload)
			//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], msgPayload)

			fmt.Println(len(client.Hub.CustomerMessageQueue))
			fmt.Println(client.Hub.CustomerMessageQueue[data.ReceiverId])
			sendMessage(client, "customer_offline", "customer offline, message added to customer message queue")
			return
		}
		client.Hub.AddMessageToCustomerQueue(data.ReceiverId, msgPayload)
		client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, msgPayload)

		// client.Hub.CustomerMessageQueue[data.ReceiverId] = append(client.Hub.CustomerMessageQueue[data.ReceiverId], msgPayload)
		// client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], msgPayload)
		for _, v := range customer {
			sendMessage(v, "message", data)
		}
		//broadcast to all agent devices//
		for _, v := range client.Hub.GetHumanAgentById(client.HumanAgentPass.Id) {
			sendMessage(v, "message", data)
		}
		//............................................//
	} else if client.Type == "Customer" {
		humanAgent := client.Hub.GetHumanAgentById(client.HumanAgentPass.Id)
		data.SenderId = client.CustomerPass.Id
		data.ReceiverId = client.HumanAgentPass.Id
		data.ConversationId = client.HumanAgentPass.ConversationSeal
		data.ContentType = "text"
		data.CreatedAt = time.Now().String()
		data.SenderType = "Customer"
		msgPayload := &model.WSMessage{
			Type:    "message",
			Payload: data,
		}
		fmt.Println("conversation seal: ", client.HumanAgentPass.ConversationSeal)
		if len(humanAgent) == 0 {
			client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, msgPayload)
			client.Hub.AddMessageToCustomerQueue(client.CustomerPass.Id, msgPayload)
			//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], msgPayload)

			sendMessage(client, "agent_offline", "agent offline, message added to agent queue")
			return
		}
		client.Hub.AddMessageToHumanAgentQueue(client.HumanAgentPass.Id, msgPayload)
		client.Hub.AddMessageToCustomerQueue(client.CustomerPass.Id, msgPayload)

		//client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id] = append(client.Hub.HumanAgentMessageQueue[client.HumanAgentPass.Id], msgPayload)
		//client.Hub.CustomerMessageQueue[client.CustomerPass.Id] = append(client.Hub.CustomerMessageQueue[client.CustomerPass.Id], msgPayload)
		for _, v := range humanAgent {
			sendMessage(v, "message", data)
		}
		for _, v := range client.Hub.GetCustomerById(client.CustomerPass.Id) {
			sendMessage(v, "message", data)
		}
	}
}

// trigger name: end_chat
// -> todos:
// *
// -> process:
// -> -> construct the payload
// -->>  validate data
// -->> find the ids
// 1. remove data from all the queues
// 2. dettach Human seal from customer
// 3. update sos flag
// '4. unmark flag
// *//
func handleEndtheChat(client *hub.Client, payload any) {
	conversation, err := constructor.ConversationPayloadConstructor(payload, false)
	if err != nil {
		log.Print("conversation construction err-> handle end chat:", err)
	}
	companyId := conversation.CompanyId
	customerId := conversation.CustomerPayload.Id
	humanAgentId := client.HumanAgentPass.Id
	conversationId := conversation.Id
	fmt.Println(companyId, customerId, humanAgentId, conversationId)
	customer := client.Hub.GetCustomerById(customerId)
	for _, v := range customer {
		v.FlagRevealed = false
		v.SosFlag = false
		v.HumanAgentPass = nil
		sendMessage(v, "end_chat", "conversation ended")
	}
}
