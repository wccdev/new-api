// chatlog-export converts chatlog archives (.jsonl.zst) into an OpenAI
// chat-format fine-tuning dataset (one {"messages": [...], "tools": [...]} object
// per line).
//
//	go run ./chatlog/cmd/chatlog-export -dir /data/chatlog -out sft.jsonl \
//	    -from 2026-09-01 -to 2026-09-30 -model gpt-4o,claude
//
// Chat APIs are stateless, so every turn of a conversation is recorded with the
// whole history. By default only the longest record of each conversation is
// exported; -keep-prefixes disables that.
package main

import (
	"bufio"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/chatlog/convert"
	"github.com/QuantumNous/new-api/common"

	"github.com/klauspost/compress/zstd"
)

type options struct {
	dir              string
	out              string
	from             string
	to               string
	models           []string
	userId           int
	keepPrefixes     bool
	includeReasoning bool
	includeMetadata  bool
}

type sample struct {
	record   convert.Record
	messages []convert.Message
	tools    []any
}

func main() {
	var opts options
	var models string
	flag.StringVar(&opts.dir, "dir", "/data/chatlog", "archive directory (copy the directories of every node into one place)")
	flag.StringVar(&opts.out, "out", "sft.jsonl", "output file")
	flag.StringVar(&opts.from, "from", "", "first day to export, YYYY-MM-DD")
	flag.StringVar(&opts.to, "to", "", "last day to export, YYYY-MM-DD")
	flag.StringVar(&models, "model", "", "comma-separated substrings; keep records whose model contains any of them")
	flag.IntVar(&opts.userId, "user", 0, "only export this user id")
	flag.BoolVar(&opts.keepPrefixes, "keep-prefixes", false, "keep records that are a prefix of a longer conversation")
	flag.BoolVar(&opts.includeReasoning, "reasoning", false, "export reasoning as reasoning_content")
	flag.BoolVar(&opts.includeMetadata, "metadata", false, "add a metadata object (request id, model, user) to every line")
	flag.Parse()
	if models != "" {
		opts.models = strings.Split(models, ",")
	}
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "chatlog-export:", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	files, err := filepath.Glob(filepath.Join(opts.dir, "dt=*", "*.jsonl.zst"))
	if err != nil {
		return err
	}
	slices.Sort(files)

	// Pass 1 collects the hash of every strict prefix of every conversation.
	prefixes := map[[sha256.Size]byte]struct{}{}
	total := 0
	err = scan(files, opts, func(s sample) error {
		total++
		hashes := requestPrefixHashes(s)
		for _, hash := range hashes[:max(len(hashes)-1, 0)] {
			prefixes[hash] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return err
	}

	output, err := os.Create(opts.out)
	if err != nil {
		return err
	}
	defer output.Close()
	writer := bufio.NewWriter(output)
	defer writer.Flush()

	// Pass 2 writes every conversation that no other record continues.
	written := 0
	seen := map[[sha256.Size]byte]struct{}{}
	err = scan(files, opts, func(s sample) error {
		hashes := requestPrefixHashes(s)
		if len(hashes) == 0 {
			return nil
		}
		whole := hashes[len(hashes)-1]
		if _, continued := prefixes[whole]; continued && !opts.keepPrefixes {
			return nil
		}
		if _, duplicate := seen[whole]; duplicate {
			return nil
		}
		seen[whole] = struct{}{}
		line, err := common.Marshal(datasetLine(s, opts))
		if err != nil {
			return err
		}
		written++
		_, err = writer.Write(append(line, '\n'))
		return err
	})
	fmt.Fprintf(os.Stderr, "usable records: %d, written: %d -> %s\n", total, written, opts.out)
	return err
}

// scan decodes every archive file and calls visit for each record that passes
// the filters and ended with a complete assistant turn.
func scan(files []string, opts options, visit func(sample) error) error {
	for _, path := range files {
		day := strings.TrimPrefix(filepath.Base(filepath.Dir(path)), "dt=")
		if (opts.from != "" && day < opts.from) || (opts.to != "" && day > opts.to) {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		decoder, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			return err
		}
		reader := bufio.NewReaderSize(decoder, 1<<20)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 1 {
				if s, ok := usableSample(line, opts); ok {
					if err := visit(s); err != nil {
						decoder.Close()
						file.Close()
						return err
					}
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "skipping rest of %s: %v\n", path, readErr)
				break
			}
		}
		decoder.Close()
		file.Close()
	}
	return nil
}

func usableSample(line []byte, opts options) (sample, bool) {
	var record convert.Record
	if err := common.Unmarshal(line, &record); err != nil {
		return sample{}, false
	}
	if record.Status != 200 || record.Truncated || record.Request == nil || record.Response == nil {
		return sample{}, false
	}
	if opts.userId != 0 && record.UserId != opts.userId {
		return sample{}, false
	}
	matchesModel := func(name string) bool { return strings.Contains(record.Model, name) }
	if len(opts.models) > 0 && !slices.ContainsFunc(opts.models, matchesModel) {
		return sample{}, false
	}
	var request map[string]any
	if common.Unmarshal(record.Request, &request) != nil {
		return sample{}, false
	}
	response := convert.DecodeResponse(record.Response)
	if !finishedNormally(record.Protocol, response) {
		return sample{}, false
	}
	messages := convert.Normalize(record.Protocol, request, response)
	if len(messages) < 2 || messages[len(messages)-1].Source != convert.SourceResponse {
		return sample{}, false
	}
	return sample{record: record, messages: messages, tools: convert.NormalizeTools(record.Protocol, request)}, true
}

// finishedNormally rejects generations cut short by length limits, content
// filters or upstream errors; they would teach the model to stop mid-answer.
func finishedNormally(protocol string, response map[string]any) bool {
	first := func(key string) map[string]any {
		items, _ := response[key].([]any)
		if len(items) == 0 {
			return nil
		}
		item, _ := items[0].(map[string]any)
		return item
	}
	switch protocol {
	case convert.ProtocolOpenAIChat, convert.ProtocolOpenAICompletions:
		reason, _ := first("choices")["finish_reason"].(string)
		return reason == "stop" || reason == "tool_calls" || reason == "function_call"
	case convert.ProtocolOpenAIResponses:
		status, _ := response["status"].(string)
		return status == "completed"
	case convert.ProtocolAnthropicMessages:
		reason, _ := response["stop_reason"].(string)
		return reason == "end_turn" || reason == "tool_use" || reason == "stop_sequence"
	case convert.ProtocolGeminiGenerate:
		reason, _ := first("candidates")["finishReason"].(string)
		return reason == "STOP"
	}
	return false
}

// requestPrefixHashes returns one running hash per client-sent message, scoped
// by user so different users never merge. The last element identifies the whole
// request; the earlier ones identify the shorter requests it continues.
func requestPrefixHashes(s sample) [][sha256.Size]byte {
	running := sha256.New()
	fmt.Fprintf(running, "user:%d\n", s.record.UserId)
	var hashes [][sha256.Size]byte
	for _, message := range s.messages {
		if message.Source != convert.SourceRequest {
			continue
		}
		fmt.Fprintf(running, "%s\x00%s\x00%s\x00", message.Role, message.Content, message.ToolCallId)
		// Clients replay assistant turns with or without reasoning and tool
		// calls, so only role and text feed the hash, and a prefix can only end
		// at a message the client authored.
		if message.Role != "assistant" {
			var sum [sha256.Size]byte
			running.Sum(sum[:0])
			hashes = append(hashes, sum)
		}
	}
	return hashes
}

func datasetLine(s sample, opts options) map[string]any {
	messages := make([]map[string]any, 0, len(s.messages))
	for _, message := range s.messages {
		entry := map[string]any{"role": message.Role, "content": message.Content}
		if opts.includeReasoning && message.Reasoning != "" {
			entry["reasoning_content"] = message.Reasoning
		}
		if message.ToolCallId != "" {
			entry["tool_call_id"] = message.ToolCallId
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{
					"id":       call.Id,
					"type":     "function",
					"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
				})
			}
			entry["tool_calls"] = calls
		}
		messages = append(messages, entry)
	}
	line := map[string]any{"messages": messages}
	if len(s.tools) > 0 {
		line["tools"] = s.tools
	}
	if opts.includeMetadata {
		line["metadata"] = map[string]any{
			"request_id": s.record.RequestId,
			"created_at": s.record.CreatedAt,
			"protocol":   s.record.Protocol,
			"model":      s.record.Model,
			"user_id":    s.record.UserId,
		}
	}
	return line
}
