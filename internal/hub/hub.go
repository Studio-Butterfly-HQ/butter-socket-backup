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
	PendingChatQueue map[string][]any //for agents map(companyid,[request list]) // unassigned
	//active queue
	ActiveChatQueue map[string][]any //agents active chats //-> inbox in front end
	//Human Agent queue
	HumanAgentMessageQueue map[string][]any

	//customer queue (customer inbox messages while not active...)
	CustomerMessageQueue map[string][]any

	CustomerEventQueue map[string][]any

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
		PendingChatQueue:       make(map[string][]any),
		ActiveChatQueue:        make(map[string][]any),
		HumanAgentMessageQueue: make(map[string][]any),
		CustomerMessageQueue:   make(map[string][]any),
		CustomerEventQueue:     make(map[string][]any),

		// accepted customers
		AcceptedCustomers: make(map[string]*model.HumanAgentPass),
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
				go h.BroadcastPendingQueue(client.HumanAgentPass.CompanyId)
				go h.BroadcastActiveChat(client.HumanAgentPass.Id)
				go h.BroadcastHumanAgentMessages(client.HumanAgentPass.Id)
			} else {
				h.customers[client.CustomerPass.Id] = append(h.customers[client.CustomerPass.Id], client)
				go h.CustomerMessageQueueBroadcast(client.CustomerPass.Id)
				go h.BroadcastCustomerEventQueue(client.CustomerPass.Id)
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
func (h *Hub) AddToPendingChat(companyID string, conversation any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.PendingChatQueue == nil {
		h.PendingChatQueue = make(map[string][]any)
	}
	wsMsgPayload := h.wsMessageCreator("transfer_chat", conversation)
	h.PendingChatQueue[companyID] = append(h.PendingChatQueue[companyID], wsMsgPayload)
}

func (h *Hub) RemoveFromPending(companyID, customerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	pendingList := h.PendingChatQueue[companyID]

	var updatedList []any

	for _, item := range pendingList {
		wsMsg, ok := item.(*model.WSMessage)
		if !ok {
			continue
		}

		conv := wsMsg.Payload.(*model.ConversationPayload)

		if conv.CustomerPayload.Id != customerID {
			updatedList = append(updatedList, conv)
		}
	}

	h.PendingChatQueue[companyID] = updatedList
}

func (h *Hub) MarkCustomerAccepted(customerID string, agent *model.HumanAgentPass) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.AcceptedCustomers[customerID] = agent
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
