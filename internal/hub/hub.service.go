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

func (h *Hub) AgentQueueBorad() {
	quedMessages, _ := json.Marshal(h.MessageQueue)
	for i, c := range h.humanAgents {
		fmt.Println("Agent Id: ", i)
		for _, v := range c {
			v.Send <- quedMessages
		}
	}
}

func (h *Hub) CustomerQueueBroadcast(id string, self *Client) {
	fmt.Println("customer queue broadcast...")
	fmt.Println(len(h.CustomerQueue))
	fmt.Println(len(h.CustomerQueue[id]))
	for _, customer := range h.customers[id] {
		for _, v := range h.CustomerQueue {
			queueByte, err := json.Marshal(v)
			if err != nil {
				fmt.Println(err)
				return
			}
			customer.Send <- queueByte
		}
	}
}
