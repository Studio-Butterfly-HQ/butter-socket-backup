package handler

import (
	"butter-time/internal/hub"
	"butter-time/internal/llm"
	"butter-time/internal/model"
	"context"
	"encoding/json"
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
	err := llm.StreamButterAI(ctx, msgIn.Content, func(token string) {
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
		return
	}
	//Tell frontend: AI finished
	sendMessage(client, "butter_typing_end", nil)
	//Save fullReply to DB ---later....---///
}
