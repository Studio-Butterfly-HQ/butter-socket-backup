package handler

import (
	"butter-time/internal/hub"
	"butter-time/internal/llm"
	"butter-time/internal/model"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func handleAiStream(client *hub.Client, payload any) {

	payloadBytes, _ := json.Marshal(payload)
	var msgIn model.MsgInOut
	json.Unmarshal(payloadBytes, &msgIn)

	//Cancel previous AI if still running
	if client.CancelAI != nil {
		client.CancelAI()
	}

	//Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	client.CancelAI = cancel

	//Tell frontend: AI started typing
	sendMessage(client, "butter_typing_start", nil)
	var fullReply string
	//Start streaming AI
	_, err := llm.RetrieveAndAnswer(ctx, client.HumanAgentPass.CompanyId, msgIn.Content, func(token string) {
		fullReply += token
		//Send token immediately
		sendMessage(client, "butter_stream", model.MsgInOut{
			SenderType:  "AI-AGENT",
			Content:     token,
			ContentType: "text",
			CreatedAt:   time.Now().Format(time.RFC3339),
		})
	})

	if err != nil {
		sendError(client, "AI error")
		sendMessage(client, "message", "ai is unavailable to response")
		fmt.Println(err.Error())
		return
	}
	//Tell frontend: AI finished
	sendMessage(client, "butter_stream_full_reply", fullReply)
	sendMessage(client, "butter_typing_end", nil)
	//Save fullReply to DB ---later....---///
}
