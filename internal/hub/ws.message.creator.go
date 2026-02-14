package hub

import (
	"butter-time/internal/model"
)

func (h *Hub) wsMessageCreator(msgType string, payload any) model.WSMessage {
	wsMsg := model.WSMessage{
		Type:    msgType,
		Payload: payload,
	}

	return wsMsg
}
