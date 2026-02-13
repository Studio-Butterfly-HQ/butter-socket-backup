package constructor

import (
	"butter-time/internal/model"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func ConversationPayloadConstructor(payload any, accept bool) (model.ConversationPayload, error) {
	var conversation model.ConversationPayload
	conversationByte, err := json.Marshal(payload)
	if err != nil {
		return model.ConversationPayload{}, err
	}
	err = json.Unmarshal(conversationByte, &conversation)
	if err != nil {
		return model.ConversationPayload{}, err
	}
	//checkpost of conversation payload// (todo)
	//---logics for the conversation payload is authentic---//
	//required: companyid, customer id and info,last (1-n [1<n<50] customer message)
	//next---->
	//construction of conversation.....................|---:<)
	conversation.Id = uuid.New().String()
	conversation.Status = "waiting"
	conversation.Summary = "About tiny super processor"
	conversation.Tags = []string{"innovation", "hardware"}
	conversation.Department = &model.Department{
		DepartmentName: "Team Alpha",
		DepartmentID:   "1",
	}
	conversation.MetaData.CreatedAt = time.Stamp
	conversation.MetaData.LastUpdated = time.Stamp
	return conversation, nil
}
