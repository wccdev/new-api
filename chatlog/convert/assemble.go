// Package convert turns recorded relay traffic into protocol-independent data:
// it rebuilds a complete response object from an SSE stream and normalizes
// requests/responses of every supported protocol into OpenAI-style messages.
// It depends only on the host JSON wrapper so offline tools can reuse it.
package convert

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ProtocolOpenAIChat        = "openai.chat"
	ProtocolOpenAICompletions = "openai.completions"
	ProtocolOpenAIResponses   = "openai.responses"
	ProtocolAnthropicMessages = "anthropic.messages"
	ProtocolGeminiGenerate    = "gemini.generate"
)

var ErrNoStreamPayload = errors.New("stream contains no usable payload")

// ProtocolForPath maps a relay request path to the recorded protocol. An empty
// result means the path is not a conversation endpoint and must not be recorded.
func ProtocolForPath(path string) string {
	switch path {
	case "/v1/chat/completions", "/pg/chat/completions":
		return ProtocolOpenAIChat
	case "/v1/completions":
		return ProtocolOpenAICompletions
	case "/v1/responses":
		return ProtocolOpenAIResponses
	case "/v1/messages":
		return ProtocolAnthropicMessages
	}
	isGeminiModelPath := strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/")
	isGenerate := strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent")
	if isGeminiModelPath && isGenerate {
		return ProtocolGeminiGenerate
	}
	return ""
}

type sseEvent struct {
	name string
	data []byte
}

func parseSSE(raw []byte) []sseEvent {
	var events []sseEvent
	var current sseEvent
	flush := func() {
		if len(current.data) > 0 {
			events = append(events, current)
		}
		current = sseEvent{}
	}
	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			flush()
			continue
		}
		if value, ok := bytes.CutPrefix(line, []byte("event:")); ok {
			current.name = string(bytes.TrimSpace(value))
			continue
		}
		if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			value = bytes.TrimPrefix(value, []byte(" "))
			if len(current.data) > 0 {
				current.data = append(current.data, '\n')
			}
			current.data = append(current.data, value...)
		}
	}
	flush()
	return events
}

// AssembleStream rebuilds the non-streaming response object a client would have
// received for the same request, from the raw SSE bytes written to it.
func AssembleStream(protocol string, raw []byte) (map[string]any, error) {
	var chunks []map[string]any
	for _, event := range parseSSE(raw) {
		if bytes.Equal(event.data, []byte("[DONE]")) {
			continue
		}
		var chunk map[string]any
		if err := common.Unmarshal(event.data, &chunk); err != nil {
			continue
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return nil, ErrNoStreamPayload
	}
	switch protocol {
	case ProtocolOpenAIChat:
		return assembleOpenAIChat(chunks), nil
	case ProtocolOpenAICompletions:
		return assembleOpenAICompletions(chunks), nil
	case ProtocolOpenAIResponses:
		return assembleOpenAIResponses(chunks)
	case ProtocolAnthropicMessages:
		return assembleAnthropic(chunks)
	case ProtocolGeminiGenerate:
		return MergeGeminiChunks(chunks), nil
	}
	return nil, errors.New("unsupported protocol: " + protocol)
}

func copyStreamEnvelope(result map[string]any, chunk map[string]any) {
	for _, key := range []string{"id", "model", "created", "system_fingerprint"} {
		if value, ok := chunk[key]; ok && value != nil && result[key] == nil {
			result[key] = value
		}
	}
	if usage, ok := chunk["usage"].(map[string]any); ok {
		result["usage"] = usage
	}
}

type openAIToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type openAIChoice struct {
	role         string
	content      strings.Builder
	reasoning    strings.Builder
	toolCalls    map[int]*openAIToolCall
	toolOrder    []int
	finishReason any
}

func assembleOpenAIChat(chunks []map[string]any) map[string]any {
	result := map[string]any{"object": "chat.completion"}
	choices := map[int]*openAIChoice{}
	var choiceOrder []int
	for _, chunk := range chunks {
		copyStreamEnvelope(result, chunk)
		for _, rawChoice := range asSlice(chunk["choices"]) {
			choiceMap := asMap(rawChoice)
			index := asInt(choiceMap["index"])
			choice, ok := choices[index]
			if !ok {
				choice = &openAIChoice{role: "assistant", toolCalls: map[int]*openAIToolCall{}}
				choices[index] = choice
				choiceOrder = append(choiceOrder, index)
			}
			if reason := choiceMap["finish_reason"]; reason != nil {
				choice.finishReason = reason
			}
			delta := asMap(choiceMap["delta"])
			if role := asString(delta["role"]); role != "" {
				choice.role = role
			}
			choice.content.WriteString(asString(delta["content"]))
			choice.reasoning.WriteString(asString(delta["reasoning_content"]))
			choice.reasoning.WriteString(asString(delta["reasoning"]))
			for _, rawCall := range asSlice(delta["tool_calls"]) {
				callMap := asMap(rawCall)
				callIndex := asInt(callMap["index"])
				call, ok := choice.toolCalls[callIndex]
				if !ok {
					call = &openAIToolCall{}
					choice.toolCalls[callIndex] = call
					choice.toolOrder = append(choice.toolOrder, callIndex)
				}
				if id := asString(callMap["id"]); id != "" {
					call.id = id
				}
				function := asMap(callMap["function"])
				if name := asString(function["name"]); name != "" {
					call.name = name
				}
				call.arguments.WriteString(asString(function["arguments"]))
			}
		}
	}
	assembled := make([]any, 0, len(choiceOrder))
	for _, index := range choiceOrder {
		choice := choices[index]
		message := map[string]any{"role": choice.role, "content": choice.content.String()}
		if choice.reasoning.Len() > 0 {
			message["reasoning_content"] = choice.reasoning.String()
		}
		if len(choice.toolOrder) > 0 {
			calls := make([]any, 0, len(choice.toolOrder))
			for _, callIndex := range choice.toolOrder {
				call := choice.toolCalls[callIndex]
				calls = append(calls, map[string]any{
					"id":   call.id,
					"type": "function",
					"function": map[string]any{
						"name":      call.name,
						"arguments": call.arguments.String(),
					},
				})
			}
			message["tool_calls"] = calls
		}
		assembled = append(assembled, map[string]any{
			"index":         index,
			"message":       message,
			"finish_reason": choice.finishReason,
		})
	}
	result["choices"] = assembled
	return result
}

func assembleOpenAICompletions(chunks []map[string]any) map[string]any {
	result := map[string]any{"object": "text_completion"}
	texts := map[int]*strings.Builder{}
	finishReasons := map[int]any{}
	var order []int
	for _, chunk := range chunks {
		copyStreamEnvelope(result, chunk)
		for _, rawChoice := range asSlice(chunk["choices"]) {
			choiceMap := asMap(rawChoice)
			index := asInt(choiceMap["index"])
			if _, ok := texts[index]; !ok {
				texts[index] = &strings.Builder{}
				order = append(order, index)
			}
			texts[index].WriteString(asString(choiceMap["text"]))
			if reason := choiceMap["finish_reason"]; reason != nil {
				finishReasons[index] = reason
			}
		}
	}
	choices := make([]any, 0, len(order))
	for _, index := range order {
		choices = append(choices, map[string]any{
			"index":         index,
			"text":          texts[index].String(),
			"finish_reason": finishReasons[index],
		})
	}
	result["choices"] = choices
	return result
}

// The Responses API repeats the full response object in its terminal event, so
// the stream needs no delta merging.
func assembleOpenAIResponses(chunks []map[string]any) (map[string]any, error) {
	terminalTypes := []string{"response.completed", "response.incomplete", "response.failed"}
	for i := len(chunks) - 1; i >= 0; i-- {
		eventType := asString(chunks[i]["type"])
		response := asMap(chunks[i]["response"])
		if len(response) > 0 && slices.Contains(terminalTypes, eventType) {
			return response, nil
		}
	}
	return nil, ErrNoStreamPayload
}

func assembleAnthropic(chunks []map[string]any) (map[string]any, error) {
	var message map[string]any
	blocks := map[int]map[string]any{}
	partialInputs := map[int]*strings.Builder{}
	var blockOrder []int
	for _, chunk := range chunks {
		switch asString(chunk["type"]) {
		case "message_start":
			message = asMap(chunk["message"])
		case "content_block_start":
			index := asInt(chunk["index"])
			blocks[index] = asMap(chunk["content_block"])
			blockOrder = append(blockOrder, index)
		case "content_block_delta":
			block := blocks[asInt(chunk["index"])]
			if block == nil {
				continue
			}
			delta := asMap(chunk["delta"])
			switch asString(delta["type"]) {
			case "text_delta":
				block["text"] = asString(block["text"]) + asString(delta["text"])
			case "thinking_delta":
				block["thinking"] = asString(block["thinking"]) + asString(delta["thinking"])
			case "signature_delta":
				block["signature"] = asString(block["signature"]) + asString(delta["signature"])
			case "input_json_delta":
				index := asInt(chunk["index"])
				if partialInputs[index] == nil {
					partialInputs[index] = &strings.Builder{}
				}
				partialInputs[index].WriteString(asString(delta["partial_json"]))
			}
		case "message_delta":
			if message == nil {
				continue
			}
			for key, value := range asMap(chunk["delta"]) {
				message[key] = value
			}
			usage := asMap(message["usage"])
			if usage == nil {
				usage = map[string]any{}
			}
			for key, value := range asMap(chunk["usage"]) {
				usage[key] = value
			}
			message["usage"] = usage
		case "error":
			return chunk, nil
		}
	}
	if message == nil {
		return nil, ErrNoStreamPayload
	}
	content := make([]any, 0, len(blockOrder))
	for _, index := range blockOrder {
		block := blocks[index]
		if partial := partialInputs[index]; partial != nil && partial.Len() > 0 {
			var input any
			if err := common.UnmarshalJsonStr(partial.String(), &input); err == nil {
				block["input"] = input
			} else {
				block["input_raw"] = partial.String()
			}
		}
		content = append(content, block)
	}
	message["content"] = content
	return message, nil
}

// MergeGeminiChunks folds streamed GenerateContentResponse chunks into one
// response. Adjacent text parts of the same kind (thought or answer) are joined.
func MergeGeminiChunks(chunks []map[string]any) map[string]any {
	result := map[string]any{}
	var parts []any
	role := "model"
	var finishReason any
	for _, chunk := range chunks {
		for key, value := range chunk {
			if key != "candidates" {
				result[key] = value
			}
		}
		candidates := asSlice(chunk["candidates"])
		if len(candidates) == 0 {
			continue
		}
		candidate := asMap(candidates[0])
		if reason := candidate["finishReason"]; reason != nil {
			finishReason = reason
		}
		content := asMap(candidate["content"])
		if value := asString(content["role"]); value != "" {
			role = value
		}
		for _, rawPart := range asSlice(content["parts"]) {
			part := asMap(rawPart)
			text, isText := part["text"].(string)
			if isText && len(parts) > 0 {
				previous := asMap(parts[len(parts)-1])
				previousText, previousIsText := previous["text"].(string)
				if previousIsText && asBool(previous["thought"]) == asBool(part["thought"]) {
					previous["text"] = previousText + text
					continue
				}
			}
			parts = append(parts, part)
		}
	}
	candidate := map[string]any{
		"index":   0,
		"content": map[string]any{"role": role, "parts": parts},
	}
	if finishReason != nil {
		candidate["finishReason"] = finishReason
	}
	result["candidates"] = []any{candidate}
	return result
}

// DecodeResponse parses a recorded response body. Gemini streaming without
// alt=sse answers with a JSON array of chunks, which is merged into one object.
func DecodeResponse(raw []byte) map[string]any {
	if common.GetJsonType(raw) == "array" {
		var chunks []map[string]any
		_ = common.Unmarshal(raw, &chunks)
		return MergeGeminiChunks(chunks)
	}
	var response map[string]any
	_ = common.Unmarshal(raw, &response)
	return response
}
