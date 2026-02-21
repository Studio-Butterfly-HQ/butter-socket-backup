package hub

import (
	"butter-time/internal/model"
	"context"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	Hub            *Hub
	Type           string
	Conn           *websocket.Conn
	Send           chan []byte
	CustomerPass   *model.CustomerPass
	HumanAgentPass *model.HumanAgentPass
	CancelAI       context.CancelFunc
	SosFlag        bool // -> true when customer talking to human or need to talk to human
	FlagRevealed   bool // -> when a human accepts connection
}

type Hub struct {
	customers   map[string][]*Client
	humanAgents map[string][]*Client

	//channels:
	register   chan *Client
	unregister chan *Client
	command    chan string

	//company->department->human-agent
	company map[string]map[string]map[string]bool

	//queues (pending messages (chat request list for company))
	//PendingChatQueue map[string][]any //for agents map(companyid,[request list]) // unassigned
	PendingChatQueue map[string]map[string]model.ConversationPayload
	//active queue
	ActiveChatQueue map[string][]any //agents active chats //-> inbox in front end
	//Human Agent queue
	HumanAgentMessageQueue map[string][]any

	//customer queue (customer inbox messages while not active...)
	CustomerMessageQueue map[string][]any

	CustomerEventQueue map[string][]any

	//sos status
	SosStatus map[string]bool
	//customer connection accept flag
	AcceptedCustomers map[string]*model.HumanAgentPass
	//thread safety
	mu sync.RWMutex
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		customers:   make(map[string][]*Client),
		humanAgents: make(map[string][]*Client),
		company:     make(map[string]map[string]map[string]bool),

		register:   make(chan *Client),
		unregister: make(chan *Client),
		command:    make(chan string),

		// queues
		PendingChatQueue:       make(map[string]map[string]model.ConversationPayload),
		ActiveChatQueue:        make(map[string][]any),
		HumanAgentMessageQueue: make(map[string][]any),
		CustomerMessageQueue:   make(map[string][]any),
		CustomerEventQueue:     make(map[string][]any),
		SosStatus:              make(map[string]bool),                  //---------------//sos status
		AcceptedCustomers:      make(map[string]*model.HumanAgentPass), //accespted by human agents
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if client.Type == "Human-Agent" {
				h.humanAgents[client.HumanAgentPass.Id] = append(h.humanAgents[client.HumanAgentPass.Id], client)
				h.registerAgent(client.HumanAgentPass)
				fmt.Println("company id for agent: ", client.HumanAgentPass.CompanyId)
				fmt.Println("pending list ", len(h.PendingChatQueue[client.HumanAgentPass.CompanyId]))
				fmt.Println(len(h.PendingChatQueue))
				for k, v := range h.PendingChatQueue {
					fmt.Println(k, "<->", v)
				}
				go h.BroadcastPendingQueue(client.HumanAgentPass.CompanyId, client)
				go h.BroadcastActiveChat(client.HumanAgentPass.Id)
				// go h.BroadcastHumanAgentMessages(client.HumanAgentPass.Id)
			} else {
				h.customers[client.CustomerPass.Id] = append(h.customers[client.CustomerPass.Id], client)
				fmt.Println("company id for client: ", client.CustomerPass.CompanyId)
				go h.CustomerMessageQueueBroadcast(client.CustomerPass.Id)
				go h.BroadcastCustomerEventQueue(client.CustomerPass.Id)
				fmt.Println(len(h.customers))
			}
			go h.printStats()
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			fmt.Println("unregister : ", client.Type)
			if client.Type == "Human-Agent" {
				agentID := client.HumanAgentPass.Id
				if list, ok := h.humanAgents[agentID]; ok {
					for i, c := range list {
						if c == client {
							list = append(list[:i], list[i+1:]...)
							break
						}
					}
					if len(list) == 0 {
						delete(h.humanAgents, agentID)
						h.unregisterAgent(client.HumanAgentPass)
					} else {
						h.humanAgents[agentID] = list
					}
				}
			} else if client.Type == "Customer" {
				customerID := client.CustomerPass.Id
				fmt.Println("unregister : ", customerID)
				fmt.Println(len(h.customers[customerID]))
				if list, ok := h.customers[customerID]; ok {
					for i, c := range list {
						if c == client {
							list = append(list[:i], list[i+1:]...)
							break
						}
					}
					fmt.Println(len(h.customers[customerID]))

					if len(list) == 0 {
						delete(h.customers, customerID)
					} else {
						h.customers[customerID] = list
					}
				}
				h.RemoveFromPendingUnsafe(client.CustomerPass.CompanyId, customerID)
			}
			client.Conn.Close()
			go h.printStats()
			h.mu.Unlock()
		}
	}
}

func (h *Hub) RegisterClient(client *Client) {
	fmt.Println("registering a client connection: ", client.Type)
	fmt.Println(client.SosFlag)
	fmt.Println(client.FlagRevealed)
	h.register <- client
}

func (h *Hub) UnregisterClient(client *Client) {
	fmt.Println("unregistering a client connection", client)
	h.unregister <- client
}

func (h *Hub) registerAgent(agent *model.HumanAgentPass) {
	// h.mu.Lock()
	// defer h.mu.Unlock()
	companyID := agent.CompanyId
	agentID := agent.Id

	// ensure company exists
	if _, ok := h.company[companyID]; !ok {
		h.company[companyID] = make(map[string]map[string]bool)
	}

	for _, dept := range agent.Departments {

		deptID := dept.DepartmentID

		// ensure department exists
		if _, ok := h.company[companyID][deptID]; !ok {
			h.company[companyID][deptID] = make(map[string]bool)
		}

		// insert agent
		h.company[companyID][deptID][agentID] = true
	}
}

func (h *Hub) unregisterAgent(agent *model.HumanAgentPass) {
	// h.mu.Lock()
	// defer h.mu.Unlock()

	companyID := agent.CompanyId
	agentID := agent.Id

	if company, ok := h.company[companyID]; ok {

		for deptID, agents := range company {

			delete(agents, agentID)

			// remove empty department
			if len(agents) == 0 {
				delete(company, deptID)
			}
		}

		// remove company if empty
		if len(company) == 0 {
			delete(h.company, companyID)
		}
	}
}

func (h *Hub) GetHumanAgents() []*Client {
	var list []*Client
	for _, v := range h.humanAgents {
		list = append(list, v...)
	}
	return list
}

func (h *Hub) GetCustomers() []*Client {
	var list []*Client
	for _, v := range h.customers {
		list = append(list, v...)
	}
	return list
}

func (h *Hub) GetAgentsByDepartment(companyID, deptID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []string

	if company, ok := h.company[companyID]; ok {
		if agents, ok := company[deptID]; ok {
			for id := range agents {
				result = append(result, id)
			}
		}
	}

	return result
}

func (h *Hub) GetCustomerById(id string) []*Client {
	return h.customers[id]
}

func (h *Hub) GetHumanAgentById(id string) []*Client {
	return h.humanAgents[id]
}

func (h *Hub) ShowCustomers() {
	fmt.Println(len(h.customers))
	for k, v := range h.customers {
		fmt.Println(k, "->", &v)
	}
}

// AddToPendingChat safely adds a conversation to the pending chat queue for a company
// func (h *Hub) AddToPendingChat(companyID string, conversation any) {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

//		if h.PendingChatQueue == nil {
//			h.PendingChatQueue = make(map[string]map[string]model.ConversationPayload)
//		}
//		wsMsgPayload := h.wsMessageCreator("transfer_chat", conversation)
//		h.PendingChatQueue[companyID] = append(h.PendingChatQueue[companyID], wsMsgPayload)
//	}
func (h *Hub) AddToPendingChat(companyID string, conv model.ConversationPayload) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.PendingChatQueue == nil {
		h.PendingChatQueue = make(map[string]map[string]model.ConversationPayload)
	}

	if h.PendingChatQueue[companyID] == nil {
		h.PendingChatQueue[companyID] = make(map[string]model.ConversationPayload)
	}

	h.PendingChatQueue[companyID][conv.Id] = conv
}

func (h *Hub) FindFromPendingChat(companyID string, conversationID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if companyChats, ok := h.PendingChatQueue[companyID]; ok {
		_, exists := companyChats[conversationID]
		return exists
	}

	return false
}

func (h *Hub) RemoveFromPending(companyID, conversationID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if companyChats, ok := h.PendingChatQueue[companyID]; ok {
		delete(companyChats, conversationID)

		// optional cleanup if empty
		if len(companyChats) == 0 {
			delete(h.PendingChatQueue, companyID)
		}
	}
}

func (h *Hub) RemoveFromPendingByCustomer(companyID, customerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	companyChats, ok := h.PendingChatQueue[companyID]
	if !ok {
		return
	}

	for convID, conv := range companyChats {
		if conv.CustomerPass.Id == customerID {
			delete(companyChats, convID)
		}
	}

	if len(companyChats) == 0 {
		delete(h.PendingChatQueue, companyID)
	}
}

func (h *Hub) RemoveFromPendingUnsafe(companyID, conversationID string) {

	if companyChats, ok := h.PendingChatQueue[companyID]; ok {
		delete(companyChats, conversationID)

		if len(companyChats) == 0 {
			delete(h.PendingChatQueue, companyID)
		}
	}
}

func (h *Hub) MarkCustomerAccepted(customerID string, agent *model.HumanAgentPass) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.AcceptedCustomers[customerID] = agent
}
func (h *Hub) UnMarkCustomerAccepted(customerID string, agent *model.HumanAgentPass) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.AcceptedCustomers, customerID)
}

func (h *Hub) AddToActiveChat(agentID string, conversation any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	wsMsgPayload := h.wsMessageCreator("accept_chat", conversation)
	h.ActiveChatQueue[agentID] = append(h.ActiveChatQueue[agentID], wsMsgPayload)
}

// AddMessageToCustomerQueue safely appends a message to a customer's event queue
func (h *Hub) AddEventToCustomerEventQueue(customerID string, msg any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.CustomerEventQueue == nil {
		h.CustomerEventQueue = make(map[string][]any)
	}

	h.CustomerEventQueue[customerID] = append(h.CustomerEventQueue[customerID], msg)
}

// AddMessageToCustomerQueue safely appends a message to a customer's message queue
func (h *Hub) AddMessageToCustomerQueue(customerID string, msg any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.CustomerMessageQueue == nil {
		h.CustomerMessageQueue = make(map[string][]any)
	}

	h.CustomerMessageQueue[customerID] = append(h.CustomerMessageQueue[customerID], msg)
}

// AddMessageToHumanAgentQueue safely appends a message to a human agent's message queue
func (h *Hub) AddMessageToHumanAgentQueue(agentID string, msg any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.HumanAgentMessageQueue == nil {
		h.HumanAgentMessageQueue = make(map[string][]any)
	}

	h.HumanAgentMessageQueue[agentID] = append(h.HumanAgentMessageQueue[agentID], msg)
}

// RemoveFromActiveChat removes a specific customer from agent's active chat queue
func (h *Hub) RemoveFromActiveChat(agentID string, customerID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	queue, exists := h.ActiveChatQueue[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found in active chat queue", agentID)
	}

	// Filter out conversations for this customer
	newQueue := []any{}
	found := false
	for _, item := range queue {
		if wsMsg, ok := item.(model.WSMessage); ok {
			if conversation, ok := wsMsg.Payload.(model.ConversationPayload); ok {
				if conversation.CustomerPass != nil && conversation.CustomerPass.Id != customerID {
					newQueue = append(newQueue, item)
				} else {
					found = true
				}
			}
		}
	}

	if !found {
		return fmt.Errorf("customer %s not found in agent %s active chat queue", customerID, agentID)
	}

	h.ActiveChatQueue[agentID] = newQueue

	// Clean up empty queue
	if len(h.ActiveChatQueue[agentID]) == 0 {
		delete(h.ActiveChatQueue, agentID)
	}

	return nil
}

// RemoveEventFromCustomerEventQueue removes all events for a customer (reset everything)
func (h *Hub) RemoveEventFromCustomerEventQueue(customerID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.CustomerEventQueue[customerID]; !exists {
		return fmt.Errorf("customer %s not found in event queue", customerID)
	}

	delete(h.CustomerEventQueue, customerID)
	return nil
}

// RemoveMessageFromCustomerQueue removes messages based on conversationID
func (h *Hub) RemoveMessageFromCustomerQueue(customerID string, conversationID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	queue, exists := h.CustomerMessageQueue[customerID]
	if !exists {
		return fmt.Errorf("customer %s not found in message queue", customerID)
	}

	// Filter out messages for this conversation
	newQueue := []any{}
	found := false
	for _, item := range queue {
		if wsMsg, ok := item.(model.WSMessage); ok {
			if msgData, ok := wsMsg.Payload.(model.MsgInOut); ok {
				if msgData.ConversationId != conversationID {
					newQueue = append(newQueue, item)
				} else {
					found = true
				}
			}
		}
	}

	if !found {
		return fmt.Errorf("conversation %s not found for customer %s", conversationID, customerID)
	}

	h.CustomerMessageQueue[customerID] = newQueue

	// Clean up empty queue
	if len(h.CustomerMessageQueue[customerID]) == 0 {
		delete(h.CustomerMessageQueue, customerID)
	}

	return nil
}

// RemoveMessageFromHumanAgentQueue removes specific customer info from agent's queue
func (h *Hub) RemoveMessageFromHumanAgentQueue(agentID string, customerID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	queue, exists := h.HumanAgentMessageQueue[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found in message queue", agentID)
	}

	// Filter out messages for this customer
	newQueue := []any{}
	found := false
	for _, item := range queue {
		if wsMsg, ok := item.(model.WSMessage); ok {
			if msgData, ok := wsMsg.Payload.(model.MsgInOut); ok {
				// Check if sender or receiver is this customer
				if msgData.SenderId != customerID && msgData.ReceiverId != customerID {
					newQueue = append(newQueue, item)
				} else {
					found = true
				}
			}
		}
	}

	if !found {
		return fmt.Errorf("customer %s not found in agent %s message queue", customerID, agentID)
	}

	h.HumanAgentMessageQueue[agentID] = newQueue

	// Clean up empty queue
	if len(h.HumanAgentMessageQueue[agentID]) == 0 {
		delete(h.HumanAgentMessageQueue, agentID)
	}

	return nil
}
