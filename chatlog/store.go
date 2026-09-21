package chatlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/chatlog/convert"
	"github.com/QuantumNous/new-api/common"
)

// buildRecord fills the body fields of a captured exchange: JSON bodies are
// kept verbatim, SSE streams are reassembled into the equivalent response
// object, and anything else is preserved as raw text.
func buildRecord(item *capture) Record {
	record := item.record
	if common.GetJsonType(item.requestBody) == "object" {
		record.Request = item.requestBody
	} else {
		record.RequestRaw = string(item.requestBody)
	}
	if record.Model == "" && record.Request != nil {
		var header struct {
			Model string `json:"model"`
		}
		_ = common.Unmarshal(record.Request, &header)
		record.Model = header.Model
	}

	record.Stream = strings.HasPrefix(item.contentType, "text/event-stream")
	if !record.Stream {
		jsonType := common.GetJsonType(item.responseBody)
		if jsonType == "object" || jsonType == "array" {
			record.Response = item.responseBody
		} else {
			record.ResponseRaw = string(item.responseBody)
		}
		return record
	}
	assembled, err := convert.AssembleStream(record.Protocol, item.responseBody)
	if err == nil {
		record.Response, err = common.Marshal(assembled)
	}
	if err != nil {
		record.Response = nil
		record.ResponseRaw = string(item.responseBody)
	}
	return record
}

// archive appends compressed records to an hourly file. Each record is its own
// zstd frame, so a file is a valid .jsonl.zst stream at any moment.
type archive struct {
	file *os.File
	path string
}

func (a *archive) append(createdAt int64, frame []byte) error {
	at := time.Unix(createdAt, 0)
	path := filepath.Join(settings.dir, "dt="+at.Format("2006-01-02"), at.Format("15")+"-"+settings.node+".jsonl.zst")
	if path != a.path {
		if a.file != nil {
			_ = a.file.Close()
			a.file = nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
		a.file, a.path = file, path
	}
	_, err := a.file.Write(frame)
	return err
}

func runWriter(items <-chan *capture) {
	files := &archive{}
	for item := range items {
		if err := persist(files, item); err != nil {
			common.SysError("chatlog: failed to persist record " + item.record.RequestId + ": " + err.Error())
		}
	}
}

func persist(files *archive, item *capture) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	record := buildRecord(item)
	line, err := common.Marshal(record)
	if err != nil {
		return err
	}
	frame := encoder.EncodeAll(append(line, '\n'), nil)
	fileErr := files.append(record.CreatedAt, frame)
	dbErr := recordDB().Create(&ChatRecord{
		RequestId: record.RequestId,
		UserId:    record.UserId,
		CreatedAt: record.CreatedAt,
		Payload:   frame,
	}).Error
	if fileErr != nil {
		return fileErr
	}
	return dbErr
}

// findRecord loads the newest record stored for a request id. A nil record
// with a nil error means nothing was recorded.
func findRecord(requestId string) (*Record, error) {
	var rows []ChatRecord
	err := recordDB().Where("request_id = ?", requestId).Order("id desc").Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	line, err := decoder.DecodeAll(rows[0].Payload, nil)
	if err != nil {
		return nil, err
	}
	record := &Record{}
	if err = common.Unmarshal(line, record); err != nil {
		return nil, err
	}
	return record, nil
}

func runRetention() {
	for {
		if common.IsMasterNode && settings.dbRetentionDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -settings.dbRetentionDays).Unix()
			if err := recordDB().Where("created_at < ?", cutoff).Delete(&ChatRecord{}).Error; err != nil {
				common.SysError("chatlog: database retention failed: " + err.Error())
			}
		}
		if settings.fileRetentionDays > 0 {
			cutoff := "dt=" + time.Now().AddDate(0, 0, -settings.fileRetentionDays).Format("2006-01-02")
			entries, _ := os.ReadDir(settings.dir)
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() && strings.HasPrefix(name, "dt=") && name < cutoff {
					_ = os.RemoveAll(filepath.Join(settings.dir, name))
				}
			}
		}
		time.Sleep(time.Hour)
	}
}
