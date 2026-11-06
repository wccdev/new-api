package chatlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// retentionBatchSize bounds each DELETE: rows carry multi-megabyte payloads,
// and a backlog of days removed in one statement would hold a huge
// transaction and burst WAL/binlog on the shared database.
const retentionBatchSize = 200

// retentionWindow is the local-time hour range [startHour, endHour) in which
// cleanup may run, so deletes stay out of daytime traffic. startHour greater
// than endHour wraps past midnight; 0-24 allows any hour.
type retentionWindow struct {
	startHour int
	endHour   int
}

var defaultRetentionWindow = retentionWindow{startHour: 2, endHour: 6}

// parseRetentionWindow reads an "H-H" hour range such as "2-6" or "23-5".
func parseRetentionWindow(value string) (retentionWindow, error) {
	startText, endText, found := strings.Cut(strings.TrimSpace(value), "-")
	start, startErr := strconv.Atoi(strings.TrimSpace(startText))
	end, endErr := strconv.Atoi(strings.TrimSpace(endText))
	if !found || startErr != nil || endErr != nil || start < 0 || start > 23 || end < 0 || end > 24 || start == end {
		return retentionWindow{}, fmt.Errorf("invalid retention window %q, want H-H such as 2-6", value)
	}
	return retentionWindow{startHour: start, endHour: end}, nil
}

func (w retentionWindow) contains(at time.Time) bool {
	hour := at.Hour()
	if w.startHour < w.endHour {
		return hour >= w.startHour && hour < w.endHour
	}
	return hour >= w.startHour || hour < w.endHour
}

// closesAt is the first end of the window after at.
func (w retentionWindow) closesAt(at time.Time) time.Time {
	end := time.Date(at.Year(), at.Month(), at.Day(), w.endHour, 0, 0, 0, at.Location())
	if !end.After(at) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// purgeExpiredRecords deletes viewer copies created before cutoff in id
// batches until none are left or deadline passes, and returns how many rows
// were removed. A backlog cut off by the deadline resumes in the next window.
func purgeExpiredRecords(cutoff int64, deadline time.Time) (int64, error) {
	var deleted int64
	for time.Now().Before(deadline) {
		var ids []int64
		err := recordDB().Model(&ChatRecord{}).Where("created_at < ?", cutoff).
			Order("id").Limit(retentionBatchSize).Pluck("id", &ids).Error
		if err != nil || len(ids) == 0 {
			return deleted, err
		}
		result := recordDB().Where("id IN ?", ids).Delete(&ChatRecord{})
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < retentionBatchSize {
			return deleted, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return deleted, nil
}

func runRetention() {
	for ; ; time.Sleep(time.Hour) {
		now := time.Now()
		if !settings.retentionWindow.contains(now) {
			continue
		}
		if common.IsMasterNode && settings.dbRetentionDays > 0 {
			cutoff := now.AddDate(0, 0, -settings.dbRetentionDays).Unix()
			deleted, err := purgeExpiredRecords(cutoff, settings.retentionWindow.closesAt(now))
			if err != nil {
				common.SysError("chatlog: database retention failed: " + err.Error())
			}
			if deleted > 0 {
				common.SysLog(fmt.Sprintf("chatlog: database retention removed %d records older than %d days", deleted, settings.dbRetentionDays))
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
	}
}
