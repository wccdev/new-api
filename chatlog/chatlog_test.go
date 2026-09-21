package chatlog

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/chatlog/convert"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssembleStreamAndNormalize(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		request  string
		stream   string
		want     []convert.Message
	}{
		{
			name:     "openai chat merges content, reasoning and tool call deltas",
			protocol: convert.ProtocolOpenAIChat,
			request:  `{"messages":[{"role":"developer","content":"be brief"},{"role":"user","content":[{"type":"text","text":"weather?"},{"type":"image_url","image_url":{"url":"https://img/1.png"}}]}]}`,
			stream: `data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"think"}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":"Let me "}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":"check.","tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":"{\"city\":"}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"SH\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`,
			want: []convert.Message{
				{Role: "system", Content: "be brief", Source: convert.SourceRequest},
				{Role: "user", Content: "weather?", Images: []string{"https://img/1.png"}, Source: convert.SourceRequest},
				{Role: "assistant", Content: "Let me check.", Reasoning: "think", Source: convert.SourceResponse,
					ToolCalls: []convert.ToolCall{{Id: "call_1", Name: "get_weather", Arguments: `{"city":"SH"}`}}},
			},
		},
		{
			name:     "anthropic rebuilds blocks and splits tool results into tool messages",
			protocol: convert.ProtocolAnthropicMessages,
			request:  `{"system":[{"type":"text","text":"sys"}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_0","content":"42"},{"type":"text","text":"go on"}]}]}`,
			stream: `event: message_start
data: {"type":"message_start","message":{"id":"m1","role":"assistant","content":[],"usage":{"input_tokens":5}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Done"}}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu_1","name":"calc","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}

`,
			want: []convert.Message{
				{Role: "system", Content: "sys", Source: convert.SourceRequest},
				{Role: "tool", Content: "42", ToolCallId: "tu_0", Source: convert.SourceRequest},
				{Role: "user", Content: "go on", Source: convert.SourceRequest},
				{Role: "assistant", Content: "Done", Reasoning: "hmm", Source: convert.SourceResponse,
					ToolCalls: []convert.ToolCall{{Id: "tu_1", Name: "calc", Arguments: `{"x":1}`}}},
			},
		},
		{
			name:     "responses api takes the terminal event object",
			protocol: convert.ProtocolOpenAIResponses,
			request:  `{"instructions":"sys","input":"hi"}`,
			stream: `event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"he"}

event: response.completed
data: {"type":"response.completed","response":{"id":"r1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}]}}

`,
			want: []convert.Message{
				{Role: "system", Content: "sys", Source: convert.SourceRequest},
				{Role: "user", Content: "hi", Source: convert.SourceRequest},
				{Role: "assistant", Content: "hello", Source: convert.SourceResponse},
			},
		},
		{
			name:     "gemini joins text parts and keeps thoughts as reasoning",
			protocol: convert.ProtocolGeminiGenerate,
			request:  `{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
			stream: `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hel"}]}}]}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"lo"}]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":7}}

`,
			want: []convert.Message{
				{Role: "system", Content: "sys", Source: convert.SourceRequest},
				{Role: "user", Content: "hi", Source: convert.SourceRequest},
				{Role: "assistant", Content: "Hello", Reasoning: "plan", Source: convert.SourceResponse},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := convert.AssembleStream(tt.protocol, []byte(tt.stream))
			require.NoError(t, err)
			var request map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tt.request, &request))

			assert.Equal(t, tt.want, convert.Normalize(tt.protocol, request, response))
		})
	}
}

// relayStub stands in for the relay chain behind Capture: it authenticates the
// caller, caches the body the way Distribute does and streams a reply.
func relayStub(userId int, reply string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("id", userId)
		c.Set("token_name", "dev-token")
		c.Set(common.RequestIdKey, c.GetHeader("X-Test-Request-Id"))
		_, err := common.GetRequestBody(c)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, reply)
	}
}

func TestCaptureStoresRecordForViewerAndArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:chatlog_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.IsMasterNode = true
	archiveDir := t.TempDir()
	t.Setenv("CHATLOG_ENABLED", "true")
	t.Setenv("CHATLOG_DIR", archiveDir)
	t.Setenv("CHATLOG_NODE", "node-a")
	t.Setenv("CHATLOG_MAX_RESP_MB", "1")

	engine := gin.New()
	engine.Use(Capture())
	// Take over the queue so records are persisted synchronously by the test.
	queue = make(chan *capture, 4)
	reply := "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	engine.POST("/v1/chat/completions", relayStub(7, reply))
	engine.POST("/v1/embeddings", relayStub(7, reply))

	post := func(path string, requestId string) {
		body := `{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"ping"}]}`
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Request-Id", requestId)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, reply, recorder.Body.String(), "client must receive the untouched stream")
	}
	post("/v1/embeddings", "req-embeddings")
	require.Empty(t, queue, "non-conversation endpoints are not recorded")
	post("/v1/chat/completions", "req-chat")
	require.Len(t, queue, 1)
	require.NoError(t, persist(&archive{}, <-queue))

	view := func(requestId string, userId int, isAdmin bool) string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "request_id", Value: requestId}}
		c.Set("id", userId)
		Conversation(c, isAdmin)
		return recorder.Body.String()
	}
	owner := view("req-chat", 7, false)
	assert.Contains(t, owner, `"model":"gpt-test"`)
	assert.Contains(t, owner, `"stream":true`)
	assert.Contains(t, owner, `{"role":"user","content":"ping","source":"request"}`)
	assert.Contains(t, owner, `{"role":"assistant","content":"pong","source":"response"}`)
	assert.Contains(t, view("req-chat", 1, true), `"content":"pong"`)
	assert.JSONEq(t, `{"success":true,"message":"","data":null}`, view("req-chat", 8, false), "other users cannot open the record")
	assert.JSONEq(t, `{"success":true,"message":"","data":null}`, view("req-missing", 1, true))

	files, err := filepath.Glob(filepath.Join(archiveDir, "dt=*", "*-node-a.jsonl.zst"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	compressed, err := os.ReadFile(files[0])
	require.NoError(t, err)
	reader, err := zstd.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	defer reader.Close()
	lines, err := io.ReadAll(reader)
	require.NoError(t, err)
	var archived Record
	require.NoError(t, common.Unmarshal(bytes.TrimSpace(lines), &archived))
	assert.Equal(t, "req-chat", archived.RequestId)
	assert.Equal(t, "dev-token", archived.TokenName)
	assert.JSONEq(t, `{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"ping"}]}`, string(archived.Request))

	// A response larger than the cap is cut, flagged and kept as raw text.
	oversized := "data: " + strings.Repeat("x", 2<<20) + "\n\n"
	engine.POST("/v1/messages", relayStub(7, oversized))
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-test","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Request-Id", "req-big")
	engine.ServeHTTP(httptest.NewRecorder(), request)
	truncated := buildRecord(<-queue)
	assert.True(t, truncated.Truncated)
	assert.Nil(t, truncated.Response)
	assert.Len(t, truncated.ResponseRaw, 1<<20)
}
