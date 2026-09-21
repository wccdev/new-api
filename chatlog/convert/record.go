package convert

import "encoding/json"

const RecordSchemaVersion = 1

// Record is one line of the archive files and the payload of a chat_records
// row. Request and Response hold the client-facing JSON untouched; the *Raw
// fields are used instead when the body is not valid JSON (truncated or a
// stream that could not be reassembled).
type Record struct {
	Schema      int             `json:"schema"`
	RequestId   string          `json:"request_id"`
	CreatedAt   int64           `json:"created_at"`
	LatencyMs   int64           `json:"latency_ms"`
	Protocol    string          `json:"protocol"`
	Path        string          `json:"path"`
	Stream      bool            `json:"stream"`
	Status      int             `json:"status"`
	UserId      int             `json:"user_id"`
	Username    string          `json:"username"`
	TokenId     int             `json:"token_id"`
	TokenName   string          `json:"token_name"`
	Group       string          `json:"group"`
	ChannelId   int             `json:"channel_id"`
	Model       string          `json:"model"`
	ClientIp    string          `json:"client_ip,omitempty"`
	UserAgent   string          `json:"user_agent,omitempty"`
	Node        string          `json:"node,omitempty"`
	Truncated   bool            `json:"truncated"`
	Request     json.RawMessage `json:"request,omitempty"`
	RequestRaw  string          `json:"request_raw,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`
	ResponseRaw string          `json:"response_raw,omitempty"`
}
