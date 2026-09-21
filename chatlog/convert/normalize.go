package convert

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	SourceRequest  = "request"
	SourceResponse = "response"
)

type ToolCall struct {
	Id        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is the protocol-independent form of one conversation turn. Source
// tells whether the turn was sent by the client or produced by the model in
// the recorded response.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallId string     `json:"tool_call_id,omitempty"`
	Images     []string   `json:"images,omitempty"`
	Source     string     `json:"source"`
}

func (m Message) isEmpty() bool {
	return m.Content == "" && m.Reasoning == "" && len(m.ToolCalls) == 0 && len(m.Images) == 0
}

// Normalize converts a recorded request and its (reassembled) response into
// OpenAI-style messages. Either side may be nil.
func Normalize(protocol string, request map[string]any, response map[string]any) []Message {
	var messages []Message
	switch protocol {
	case ProtocolOpenAIChat:
		for _, raw := range asSlice(request["messages"]) {
			messages = append(messages, openAIChatMessage(asMap(raw), SourceRequest))
		}
		if choices := asSlice(response["choices"]); len(choices) > 0 {
			messages = append(messages, openAIChatMessage(asMap(asMap(choices[0])["message"]), SourceResponse))
		}
	case ProtocolOpenAICompletions:
		prompt := asString(request["prompt"])
		if prompt == "" {
			prompt = joinTexts(asSlice(request["prompt"]))
		}
		messages = append(messages, Message{Role: "user", Content: prompt, Source: SourceRequest})
		if choices := asSlice(response["choices"]); len(choices) > 0 {
			messages = append(messages, Message{Role: "assistant", Content: asString(asMap(choices[0])["text"]), Source: SourceResponse})
		}
	case ProtocolOpenAIResponses:
		if instructions := asString(request["instructions"]); instructions != "" {
			messages = append(messages, Message{Role: "system", Content: instructions, Source: SourceRequest})
		}
		if input := asString(request["input"]); input != "" {
			messages = append(messages, Message{Role: "user", Content: input, Source: SourceRequest})
		}
		messages = append(messages, responsesItems(asSlice(request["input"]), SourceRequest)...)
		messages = append(messages, responsesItems(asSlice(response["output"]), SourceResponse)...)
	case ProtocolAnthropicMessages:
		system := asString(request["system"])
		if system == "" {
			system = joinTexts(asSlice(request["system"]))
		}
		if system != "" {
			messages = append(messages, Message{Role: "system", Content: system, Source: SourceRequest})
		}
		for _, raw := range asSlice(request["messages"]) {
			message := asMap(raw)
			messages = append(messages, anthropicMessages(asString(message["role"]), message["content"], SourceRequest)...)
		}
		if asString(response["type"]) != "error" {
			messages = append(messages, anthropicMessages("assistant", response["content"], SourceResponse)...)
		}
	case ProtocolGeminiGenerate:
		systemInstruction := request["systemInstruction"]
		if systemInstruction == nil {
			systemInstruction = request["system_instruction"]
		}
		if system := joinTexts(asSlice(asMap(systemInstruction)["parts"])); system != "" {
			messages = append(messages, Message{Role: "system", Content: system, Source: SourceRequest})
		}
		for _, raw := range asSlice(request["contents"]) {
			messages = append(messages, geminiMessages(asMap(raw), SourceRequest)...)
		}
		if candidates := asSlice(response["candidates"]); len(candidates) > 0 {
			messages = append(messages, geminiMessages(asMap(asMap(candidates[0])["content"]), SourceResponse)...)
		}
	}
	kept := messages[:0]
	for _, message := range messages {
		if !message.isEmpty() || message.Role == "tool" {
			kept = append(kept, message)
		}
	}
	return kept
}

func openAIChatMessage(raw map[string]any, source string) Message {
	message := Message{Role: asString(raw["role"]), Source: source, ToolCallId: asString(raw["tool_call_id"])}
	if message.Role == "developer" {
		message.Role = "system"
	}
	if message.Role == "" {
		message.Role = "assistant"
	}
	message.Reasoning = asString(raw["reasoning_content"])
	if message.Reasoning == "" {
		message.Reasoning = asString(raw["reasoning"])
	}
	if text, ok := raw["content"].(string); ok {
		message.Content = text
	}
	var texts []string
	for _, rawPart := range asSlice(raw["content"]) {
		part := asMap(rawPart)
		switch asString(part["type"]) {
		case "text", "input_text", "output_text":
			texts = append(texts, asString(part["text"]))
		case "image_url":
			url := asString(asMap(part["image_url"])["url"])
			if url == "" {
				url = asString(part["image_url"])
			}
			message.Images = append(message.Images, url)
		}
	}
	if len(texts) > 0 {
		message.Content = strings.Join(texts, "\n")
	}
	for _, rawCall := range asSlice(raw["tool_calls"]) {
		call := asMap(rawCall)
		function := asMap(call["function"])
		message.ToolCalls = append(message.ToolCalls, ToolCall{
			Id:        asString(call["id"]),
			Name:      asString(function["name"]),
			Arguments: asString(function["arguments"]),
		})
	}
	return message
}

func responsesItems(items []any, source string) []Message {
	var messages []Message
	for _, rawItem := range items {
		item := asMap(rawItem)
		itemType := asString(item["type"])
		switch {
		case itemType == "function_call":
			messages = append(messages, Message{Role: "assistant", Source: source, ToolCalls: []ToolCall{{
				Id:        asString(item["call_id"]),
				Name:      asString(item["name"]),
				Arguments: asString(item["arguments"]),
			}}})
		case itemType == "function_call_output":
			messages = append(messages, Message{Role: "tool", Source: source, ToolCallId: asString(item["call_id"]), Content: textOrJSON(item["output"])})
		case itemType == "reasoning":
			reasoning := joinTexts(asSlice(item["summary"]))
			if reasoning == "" {
				reasoning = joinTexts(asSlice(item["content"]))
			}
			messages = append(messages, Message{Role: "assistant", Source: source, Reasoning: reasoning})
		case itemType == "message" || item["role"] != nil:
			messages = append(messages, openAIChatMessage(map[string]any{"role": item["role"], "content": item["content"]}, source))
			for _, rawPart := range asSlice(item["content"]) {
				part := asMap(rawPart)
				if asString(part["type"]) == "input_image" {
					last := &messages[len(messages)-1]
					last.Images = append(last.Images, asString(part["image_url"]))
				}
			}
		}
	}
	return messages
}

// An Anthropic turn may mix text with tool results; tool results become their
// own tool-role messages so the output matches the OpenAI conversation shape.
func anthropicMessages(role string, content any, source string) []Message {
	main := Message{Role: role, Source: source}
	if text, ok := content.(string); ok {
		main.Content = text
		return []Message{main}
	}
	var texts []string
	var toolResults []Message
	for _, rawBlock := range asSlice(content) {
		block := asMap(rawBlock)
		switch asString(block["type"]) {
		case "text":
			texts = append(texts, asString(block["text"]))
		case "thinking":
			main.Reasoning += asString(block["thinking"])
		case "image":
			imageSource := asMap(block["source"])
			if url := asString(imageSource["url"]); url != "" {
				main.Images = append(main.Images, url)
			} else if data := asString(imageSource["data"]); data != "" {
				main.Images = append(main.Images, "data:"+asString(imageSource["media_type"])+";base64,"+data)
			}
		case "tool_use", "server_tool_use":
			main.ToolCalls = append(main.ToolCalls, ToolCall{
				Id:        asString(block["id"]),
				Name:      asString(block["name"]),
				Arguments: textOrJSON(block["input"]),
			})
		case "tool_result":
			result := asString(block["content"])
			if result == "" {
				result = joinTexts(asSlice(block["content"]))
			}
			toolResults = append(toolResults, Message{Role: "tool", Source: source, ToolCallId: asString(block["tool_use_id"]), Content: result})
		}
	}
	main.Content = strings.Join(texts, "\n")
	if main.isEmpty() {
		return toolResults
	}
	return append(toolResults, main)
}

func geminiMessages(content map[string]any, source string) []Message {
	role := "user"
	if asString(content["role"]) == "model" {
		role = "assistant"
	}
	main := Message{Role: role, Source: source}
	var texts []string
	var toolResults []Message
	for _, rawPart := range asSlice(content["parts"]) {
		part := asMap(rawPart)
		if text, ok := part["text"].(string); ok {
			if asBool(part["thought"]) {
				main.Reasoning += text
			} else {
				texts = append(texts, text)
			}
		}
		inlineData := asMap(part["inlineData"])
		if inlineData == nil {
			inlineData = asMap(part["inline_data"])
		}
		if data := asString(inlineData["data"]); data != "" {
			mimeType := asString(inlineData["mimeType"])
			if mimeType == "" {
				mimeType = asString(inlineData["mime_type"])
			}
			main.Images = append(main.Images, "data:"+mimeType+";base64,"+data)
		}
		if call := asMap(part["functionCall"]); call != nil {
			main.ToolCalls = append(main.ToolCalls, ToolCall{Id: asString(call["id"]), Name: asString(call["name"]), Arguments: textOrJSON(call["args"])})
		}
		if result := asMap(part["functionResponse"]); result != nil {
			toolResults = append(toolResults, Message{Role: "tool", Source: source, ToolCallId: asString(result["name"]), Content: textOrJSON(result["response"])})
		}
	}
	main.Content = strings.Join(texts, "")
	if main.isEmpty() {
		return toolResults
	}
	return append(toolResults, main)
}

// NormalizeTools converts the request's tool declarations into the OpenAI
// chat-completions shape used by fine-tuning datasets.
func NormalizeTools(protocol string, request map[string]any) []any {
	var tools []any
	for _, rawTool := range asSlice(request["tools"]) {
		tool := asMap(rawTool)
		switch protocol {
		case ProtocolOpenAIChat:
			tools = append(tools, tool)
		case ProtocolOpenAIResponses:
			if asString(tool["type"]) == "function" {
				tools = append(tools, openAIFunctionTool(tool["name"], tool["description"], tool["parameters"]))
			}
		case ProtocolAnthropicMessages:
			if tool["input_schema"] != nil {
				tools = append(tools, openAIFunctionTool(tool["name"], tool["description"], tool["input_schema"]))
			}
		case ProtocolGeminiGenerate:
			declarations := asSlice(tool["functionDeclarations"])
			if declarations == nil {
				declarations = asSlice(tool["function_declarations"])
			}
			for _, rawDeclaration := range declarations {
				declaration := asMap(rawDeclaration)
				parameters := declaration["parameters"]
				if parameters == nil {
					parameters = declaration["parametersJsonSchema"]
				}
				tools = append(tools, openAIFunctionTool(declaration["name"], declaration["description"], parameters))
			}
		}
	}
	return tools
}

func openAIFunctionTool(name any, description any, parameters any) map[string]any {
	function := map[string]any{"name": name}
	if description != nil {
		function["description"] = description
	}
	if parameters != nil {
		function["parameters"] = parameters
	}
	return map[string]any{"type": "function", "function": function}
}

func asMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func asSlice(value any) []any {
	result, _ := value.([]any)
	return result
}

func asString(value any) string {
	result, _ := value.(string)
	return result
}

func asBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func asInt(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	}
	return 0
}

// joinTexts concatenates the "text" fields of content parts; plain strings in
// the list are kept as they are.
func joinTexts(parts []any) string {
	var texts []string
	for _, rawPart := range parts {
		if text, ok := rawPart.(string); ok {
			texts = append(texts, text)
			continue
		}
		if text := asString(asMap(rawPart)["text"]); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func textOrJSON(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
