package chatlog

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/chatlog/convert"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// capture is everything taken from a finished request. It is built on the
// request goroutine and turned into a Record by the writer goroutine, so JSON
// work and I/O never delay the client.
type capture struct {
	record       Record
	requestBody  []byte
	responseBody []byte
	contentType  string
}

var droppedCaptures atomic.Int64

// teeWriter copies the bytes sent to the client into a bounded buffer. Flush
// and Hijack come from the embedded writer, so streaming is unaffected.
type teeWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	limit     int
	truncated bool
}

func (w *teeWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.body.Len()
	if remaining >= len(data) {
		w.body.Write(data)
	} else {
		w.body.Write(data[:max(remaining, 0)])
		w.truncated = true
	}
	return w.ResponseWriter.Write(data)
}

func (w *teeWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// Capture records conversation relay calls. It must be registered inside
// BodyStorageCleanup so the cached request body is still readable once the
// handler returns.
func Capture() gin.HandlerFunc {
	setup()
	return func(c *gin.Context) {
		if !settings.enabled || c.Request.Method != "POST" {
			c.Next()
			return
		}
		protocol := convert.ProtocolForPath(c.Request.URL.Path)
		if protocol == "" || !strings.HasPrefix(c.ContentType(), "application/json") {
			c.Next()
			return
		}
		startedAt := time.Now()
		writer := &teeWriter{ResponseWriter: c.Writer, limit: settings.maxResponseBytes}
		c.Writer = writer
		c.Next()

		userId := c.GetInt("id")
		storage, hasBody := c.Get(common.KeyBodyStorage)
		bodyStorage, _ := storage.(common.BodyStorage)
		// No user or no cached body means the request was rejected before relay.
		if userId == 0 || !hasBody || bodyStorage == nil {
			return
		}
		item := &capture{
			responseBody: writer.body.Bytes(),
			contentType:  writer.Header().Get("Content-Type"),
			record: Record{
				Schema:    recordSchemaVersion,
				RequestId: c.GetString(common.RequestIdKey),
				CreatedAt: startedAt.Unix(),
				LatencyMs: time.Since(startedAt).Milliseconds(),
				Protocol:  protocol,
				Path:      c.Request.URL.Path,
				Status:    writer.Status(),
				UserId:    userId,
				Username:  c.GetString("username"),
				TokenId:   c.GetInt("token_id"),
				TokenName: c.GetString("token_name"),
				Group:     common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
				ChannelId: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
				Model:     common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
				ClientIp:  c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
				Node:      settings.node,
				Truncated: writer.truncated,
			},
		}
		if bodyStorage.Size() > settings.maxRequestBytes {
			item.record.Truncated = true
		}
		reader, err := bodyStorage.NewReader()
		if err != nil {
			return
		}
		item.requestBody, err = io.ReadAll(io.LimitReader(reader, settings.maxRequestBytes))
		_ = reader.Close()
		if err != nil {
			return
		}
		select {
		case queue <- item:
		default:
			if dropped := droppedCaptures.Add(1); dropped%100 == 1 {
				common.SysError("chatlog queue full, records dropped so far: " + common.Interface2String(dropped))
			}
		}
	}
}
