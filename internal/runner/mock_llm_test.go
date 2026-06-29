package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const dummyLLMResponse = "dummy response"

type mockLLM struct {
	origin   string
	mu       sync.Mutex
	requests []mockLLMRequest
}

type mockLLMRequest struct {
	path   string
	header http.Header
	body   map[string]any
	raw    string
}

func newMockLLM(t *testing.T) *mockLLM {
	t.Helper()
	mock := &mockLLM{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(mock.handle))
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	mock.origin = "http://host.docker.internal:" + port
	t.Cleanup(server.Close)
	return mock
}

func (m *mockLLM) handle(w http.ResponseWriter, r *http.Request) {
	rawData, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(rawData, &body)
	m.mu.Lock()
	m.requests = append(m.requests, mockLLMRequest{
		path:   r.URL.Path,
		header: r.Header.Clone(),
		body:   body,
		raw:    string(rawData),
	})
	m.mu.Unlock()

	switch r.URL.Path {
	case "/v1/messages":
		writeAnthropicMessages(w, body)
	case "/v1/chat/completions":
		writeChatCompletions(w, body)
	case "/v1/responses":
		writeResponses(w, body)
	default:
		http.NotFound(w, r)
	}
}

func (m *mockLLM) request(t *testing.T, path string) mockLLMRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, request := range m.requests {
		if request.path == path {
			return request
		}
	}
	t.Fatalf("no request for %s; got %#v", path, m.requests)
	return mockLLMRequest{}
}

func writeAnthropicMessages(w http.ResponseWriter, body map[string]any) {
	model, _ := body["model"].(string)
	writeSSE(w, []sseEvent{
		{"message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1}}}},
		{"content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}},
		{"content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": dummyLLMResponse}}},
		{"content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}},
		{"message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 2}}},
		{"message_stop", map[string]any{"type": "message_stop"}},
	})
}

func writeChatCompletions(w http.ResponseWriter, body map[string]any) {
	model, _ := body["model"].(string)
	writeSSE(w, []sseEvent{
		{"", chatChunk(model, map[string]any{"role": "assistant"}, nil)},
		{"", chatChunk(model, map[string]any{"content": dummyLLMResponse}, nil)},
		{"", chatChunk(model, map[string]any{}, "stop")},
	})
}

func chatChunk(model string, delta map[string]any, finish any) map[string]any {
	return map[string]any{
		"id":      "chatcmpl_1",
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	}
}

func writeResponses(w http.ResponseWriter, body map[string]any) {
	model, _ := body["model"].(string)
	responseID := "resp_1"
	itemID := "msg_1"
	completed := map[string]any{
		"id":         responseID,
		"object":     "response",
		"created_at": 0,
		"status":     "completed",
		"model":      model,
		"output": []any{map[string]any{
			"id":      itemID,
			"type":    "message",
			"status":  "completed",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": dummyLLMResponse, "annotations": []any{}}},
		}},
		"parallel_tool_calls": false,
		"tool_choice":         "auto",
		"tools":               []any{},
		"usage":               map[string]any{"input_tokens": 1, "output_tokens": 2, "total_tokens": 3},
	}
	writeSSE(w, []sseEvent{
		{"response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": responseID, "object": "response", "created_at": 0, "status": "in_progress", "model": model, "output": []any{}, "parallel_tool_calls": false, "tool_choice": "auto", "tools": []any{}}}},
		{"response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}}},
		{"response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": itemID, "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}},
		{"response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": itemID, "output_index": 0, "content_index": 0, "delta": dummyLLMResponse}},
		{"response.output_text.done", map[string]any{"type": "response.output_text.done", "item_id": itemID, "output_index": 0, "content_index": 0, "text": dummyLLMResponse}},
		{"response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": itemID, "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": dummyLLMResponse, "annotations": []any{}}}},
		{"response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": completed["output"].([]any)[0]}},
		{"response.completed", map[string]any{"type": "response.completed", "response": completed}},
	})
}

type sseEvent struct {
	name string
	data map[string]any
}

func writeSSE(w http.ResponseWriter, events []sseEvent) {
	w.Header().Set("content-type", "text/event-stream")
	for _, event := range events {
		data, _ := json.Marshal(event.data)
		if event.name != "" {
			fmt.Fprintf(w, "event: %s\n", event.name)
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}
