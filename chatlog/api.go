package chatlog

import (
	"github.com/QuantumNous/new-api/chatlog/convert"
	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// conversationView is the viewer payload: the stored record plus its messages
// normalized to one shape for every protocol.
type conversationView struct {
	Record
	Messages []convert.Message `json:"messages"`
	Tools    []any             `json:"tools,omitempty"`
}

func newConversationView(record *Record) conversationView {
	var request map[string]any
	_ = common.Unmarshal(record.Request, &request)
	response := convert.DecodeResponse(record.Response)
	messages := convert.Normalize(record.Protocol, request, response)
	if messages == nil {
		messages = []convert.Message{}
	}
	return conversationView{
		Record:   *record,
		Messages: messages,
		Tools:    convert.NormalizeTools(record.Protocol, request),
	}
}

// Conversation answers a viewer request. Administrators can open any record;
// other users only records of their own requests. data=null means no record is
// visible to the caller, so a missing record and someone else's look the same.
func Conversation(c *gin.Context, isAdmin bool) {
	setup()
	if !settings.enabled {
		common.ApiSuccess(c, nil)
		return
	}
	record, err := findRecord(c.Param("request_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if record == nil || (!isAdmin && record.UserId != c.GetInt("id")) {
		common.ApiSuccess(c, nil)
		return
	}
	if !isAdmin {
		record.ChannelId = 0
		record.ClientIp = ""
		record.Node = ""
	}
	common.ApiSuccess(c, newConversationView(record))
}
