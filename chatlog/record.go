// Package chatlog records the full request and response of conversation relay
// calls. Every node appends records to local .jsonl.zst files (the fine-tuning
// archive) and stores a compressed copy in the shared database so any node can
// serve the conversation viewer. The feature is off unless CHATLOG_ENABLED is set.
package chatlog

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/klauspost/compress/zstd"
	"gorm.io/gorm"
)

const recordSchemaVersion = 1

// Record is one line of the archive files and the payload of a ChatRecord row.
// Request and Response hold the client-facing JSON untouched; the *Raw fields
// are used instead when the body is not valid JSON (truncated or a stream that
// could not be reassembled).
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

// ChatRecord is the viewer's copy of a Record, keyed by the request id shown in
// the usage logs. Payload is the zstd-compressed Record JSON.
type ChatRecord struct {
	Id        int64  `gorm:"primaryKey"`
	RequestId string `gorm:"type:varchar(64);index:idx_chat_records_request_id"`
	UserId    int    `gorm:"index:idx_chat_records_user_id"`
	CreatedAt int64  `gorm:"bigint;index:idx_chat_records_created_at"`
	Payload   []byte
}

type config struct {
	enabled           bool
	dir               string
	node              string
	maxRequestBytes   int64
	maxResponseBytes  int
	queueSize         int
	dbRetentionDays   int
	fileRetentionDays int
}

var (
	initOnce sync.Once
	settings config
	queue    chan *capture
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
)

// recordDB is the shared database holding ChatRecord rows. A ClickHouse log
// store cannot serve point lookups with blobs, so the primary database is used
// in that deployment.
func recordDB() *gorm.DB {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return model.DB
	}
	return model.LOG_DB
}

func loadConfig() config {
	node := common.GetEnvOrDefaultString("CHATLOG_NODE", "")
	if node == "" {
		node, _ = os.Hostname()
	}
	return config{
		enabled:           common.GetEnvOrDefaultBool("CHATLOG_ENABLED", false),
		dir:               common.GetEnvOrDefaultString("CHATLOG_DIR", "/data/chatlog"),
		node:              node,
		maxRequestBytes:   int64(common.GetEnvOrDefault("CHATLOG_MAX_REQ_MB", 16)) << 20,
		maxResponseBytes:  common.GetEnvOrDefault("CHATLOG_MAX_RESP_MB", 8) << 20,
		queueSize:         common.GetEnvOrDefault("CHATLOG_QUEUE_SIZE", 256),
		dbRetentionDays:   common.GetEnvOrDefault("CHATLOG_DB_RETENTION_DAYS", 30),
		fileRetentionDays: common.GetEnvOrDefault("CHATLOG_FILE_RETENTION_DAYS", 0),
	}
}

// setup runs once, from the Capture call made during router construction,
// which happens after the databases are initialized.
func setup() {
	initOnce.Do(func() {
		settings = loadConfig()
		if !settings.enabled {
			return
		}
		var err error
		if encoder, err = zstd.NewWriter(nil); err != nil {
			common.SysError("chatlog disabled: " + err.Error())
			settings.enabled = false
			return
		}
		if decoder, err = zstd.NewReader(nil); err != nil {
			common.SysError("chatlog disabled: " + err.Error())
			settings.enabled = false
			return
		}
		if common.IsMasterNode {
			if err = recordDB().AutoMigrate(&ChatRecord{}); err != nil {
				common.SysError("chatlog disabled, migration failed: " + err.Error())
				settings.enabled = false
				return
			}
		}
		queue = make(chan *capture, settings.queueSize)
		go runWriter(queue)
		go runRetention()
		common.SysLog("chatlog enabled, archive dir: " + settings.dir)
	})
}
