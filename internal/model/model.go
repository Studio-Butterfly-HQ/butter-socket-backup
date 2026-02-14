package model

type MetaData struct {
	CreatedAt   string `json:"created_at"`
	LastUpdated string `json:"last_updated"`
}

type Customer struct {
	MetaData
	Id        string
	Name      string
	Picture   string
	Contact   string
	Source    string
	CompanyId string
}

type HumanAgentPass struct {
	Id               string
	CompanyId        string
	Departments      []Department
	ConversationSeal string //used for assigning self for a customer
}

type CustomerPass struct {
	Id        string
	CompanyId string
}

type GuestPass struct {
	Id string
}

// api response for user data
type EssentialResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	User      User   `json:"data"`
	Timestamp string `json:"timestamp"`
	Path      string `json:"path"`
}

type User struct {
	UserID      string       `json:"userId"`
	CompanyID   string       `json:"companyId"`
	Departments []Department `json:"departments"`
}

type Department struct {
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
}

// WebSocket message types
type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"` //msg in out
}

// payload for -> trigger: message
type MsgInOut struct {
	SenderId       string `json:"sender_id"`
	SenderType     string `json:"sender_type"`
	ReceiverId     string `json:"receiver_id,omitempty"`
	ConversationId string `json:"conversation_id,omitempty"`
	Typing         string `json:"typing,omitempty"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type"`
	CreatedAt      string `json:"created_at,omitempty"`
}

// payload for -> trigger: transfer_chat ////payload for -> trigger: accept_chat//payload for -> trigger: accept_chat
type CustomerPayload struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Picture   string `json:"picture"`
	Contact   string `json:"contact"`
	Source    string `json:"source"`
	CompanyId string `json:"company_id"`
}

type ConversationPayload struct {
	MetaData         `json:"metadata"`
	Id               string `json:"id"`
	Status           string `json:"status"`
	*CustomerPayload `json:"customer"`
	Summary          string   `json:"summary"`
	Tags             []string `json:"tags"`
	Messages         []string `json:"messages"`
	*AssignedTo      `json:"assigned_to"`
	*Department      `json:"department"`
}

type ExceptionPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type TypingPayload struct {
	SenderId   string `json:"sender_id"`
	ReceiverId string `json:"receiver_id,omitempty"`
	Typing     bool   `json:"typing"`
}

type AssignedTo struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	ProfileUri string `json:"profile_uri"`
}
