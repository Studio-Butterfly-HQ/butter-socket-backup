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

	//queues
	MessageQueue []any

	//Human Agent queue
	HumanAgentQueue map[string][]any

	//customer queue
	CustomerQueue map[string][]any

	//thread safety
	mu sync.RWMutex
}

// NewHub creates a new Hub instanceinstance
func NewHub() *Hub {
	return &Hub{
		customers:       make(map[string][]*Client),
		humanAgents:     make(map[string][]*Client),
		company:         make(map[string]map[string]map[string]bool),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		command:         make(chan string),
		MessageQueue:    make([]any, 0),
		HumanAgentQueue: make(map[string][]any), // -> used by : {triggers:[message]}
		CustomerQueue:   make(map[string][]any), // -> used by : {triggers:[message]}
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
				go h.AgentQueueBorad()
			} else {
				h.customers[client.CustomerPass.Id] = append(h.customers[client.CustomerPass.Id], client)
				go h.CustomerQueueBroadcast(client.CustomerPass.Id, client)
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
