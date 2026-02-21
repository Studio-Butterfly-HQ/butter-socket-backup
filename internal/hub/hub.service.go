package hub

import (
	"encoding/json"
	"fmt"
)

func (h *Hub) printStats() {
	fmt.Println("Human-Agent client count: ", len(h.humanAgents))
	fmt.Println("Customer client count:", len(h.customers))
	fmt.Println("Device per Human client: ")
	for i, c := range h.humanAgents {
		fmt.Println("Agent Id: ", i, " -> ", len(c))
	}
}

func (h *Hub) CustomerMessageQueueBroadcast(customerID string) {
	// h.mu.RLock()
	// defer h.mu.RUnlock()

	queue, ok := h.CustomerMessageQueue[customerID]
	if !ok || len(queue) == 0 {
		return
	}

	for _, msg := range queue {
		queueBytes, err := json.Marshal(msg)
		if err != nil {
			fmt.Println("Error marshaling customer message queue:", err)
			return
		}

		devices, ok := h.customers[customerID]
		if !ok || len(devices) == 0 {
			return
		}

		for _, customer := range devices {
			select {
			case customer.Send <- queueBytes:
			default:
				fmt.Println("Customer send channel full, skipping device")
			}
		}
	}
}

func (h *Hub) BroadcastCustomerEventQueue(customerID string) {
	// h.mu.RLock()
	// defer h.mu.RUnlock()

	queue, ok := h.CustomerEventQueue[customerID]
	if !ok || len(queue) == 0 {
		return
	}

	for _, item := range queue {
		queueBytes, err := json.Marshal(item)
		if err != nil {
			fmt.Println("Error marshaling customer event queue:", err)
			return
		}

		devices, ok := h.customers[customerID]
		if !ok || len(devices) == 0 {
			return
		}

		for _, customer := range devices {
			select {
			case customer.Send <- queueBytes:
			default:
				fmt.Println("Customer send channel full, skipping device")
			}
		}
	}
}

func (h *Hub) BroadcastPendingQueue(companyID string, client *Client) {
	// h.mu.RLock()
	// defer h.mu.RUnlock()

	queue, ok := h.PendingChatQueue[companyID]
	if !ok || len(queue) == 0 {
		return
	}

	for _, conversation := range queue {
		wsMsg := h.wsMessageCreator("transfer_chat", conversation)
		msgBytes, err := json.Marshal(wsMsg)
		if err != nil {
			fmt.Println("Error marshaling pending queue:", err)
			continue
		}
		select {
		case client.Send <- msgBytes:
		default:
			fmt.Println("Agent send channel is full")
		}
	}
}

func (h *Hub) BroadcastActiveChat(agentID string) {
	// h.mu.RLock()
	// defer h.mu.RUnlock()

	queue, ok := h.ActiveChatQueue[agentID]
	if !ok || len(queue) == 0 {
		return
	}
	for _, item := range queue {
		queueBytes, err := json.Marshal(item)
		if err != nil {
			fmt.Println("Error marshaling active chat queue:", err)
			return
		}

		devices, ok := h.humanAgents[agentID]
		if !ok {
			return
		}

		for _, device := range devices {
			select {
			case device.Send <- queueBytes:
			default:
				fmt.Println("Agent send channel full, skipping device")
			}
		}
	}
}

func (h *Hub) BroadcastHumanAgentMessages(agentID string) {
	// h.mu.RLock()
	// defer h.mu.RUnlock()

	queue, ok := h.HumanAgentMessageQueue[agentID]
	if !ok || len(queue) == 0 {
		return
	}

	for _, msg := range queue {
		queueBytes, err := json.Marshal(msg)
		if err != nil {
			fmt.Println("Error marshaling human agent queue:", err)
			return
		}

		devices, ok := h.humanAgents[agentID]
		if !ok {
			return
		}

		for _, device := range devices {
			select {
			case device.Send <- queueBytes:
			default:
				fmt.Println("Agent send channel full, skipping device")
			}
		}
	}
}