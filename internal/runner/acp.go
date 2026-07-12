package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
)

const harnessMessageLimit = 16 << 20

type acpBridge struct {
	ctx     context.Context
	options Options
	image   string
	harness string
	conn    *acp.AgentSideConnection
	ready   chan struct{}

	mu       sync.Mutex
	sessions map[acp.SessionId]*acpSession
}

type acpSession struct {
	bridge    *acpBridge
	id        acp.SessionId
	container string
	cwd       string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	scanner   *bufio.Scanner
	stderr    *prefixBuffer
	done      chan error
	threadID  string
	turnID    string

	writeMu sync.Mutex
	mu      sync.Mutex
	active  bool
	cancel  bool
	stop    sync.Once
}

type acpTurn struct {
	seen      map[int]bool
	tools     map[int]*claudeTool
	requestID string
	stop      acp.StopReason
	err       string
}

type claudeTool struct {
	id      string
	partial strings.Builder
}

type prefixBuffer struct{ bytes.Buffer }

func (b *prefixBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - b.Len(); remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
}

func runACP(ctx context.Context, options Options) (int, error) {
	if options.Prompt != nil {
		return 1, fmt.Errorf("--prompt cannot be used with --acp")
	}
	if options.Workspace != "" {
		return 1, fmt.Errorf("--workspace cannot be used with --acp; the ACP client supplies the workspace")
	}
	harness := options.Harness
	if options.Project != nil {
		harness = options.Project.AgentFile.Spec.Harness.Name()
	}
	if harness == "" {
		return 1, fmt.Errorf("image %q predates interactive support (missing build.agentfile.harness label); rebuild it with a current af", options.Image)
	}
	if harness != "claudecode" && harness != "codex" && harness != "pi" {
		return 1, fmt.Errorf("--acp does not support harness %s", harness)
	}
	image, err := resolveRunImage(ctx, options)
	if err != nil {
		return 1, err
	}
	bridge := &acpBridge{
		ctx:      ctx,
		options:  options,
		image:    image,
		harness:  harness,
		ready:    make(chan struct{}),
		sessions: map[acp.SessionId]*acpSession{},
	}
	conn := acp.NewAgentSideConnection(bridge, options.Stdout, options.Stdin)
	bridge.conn = conn
	close(bridge.ready)

	select {
	case <-conn.Done():
		bridge.closeAll()
		return 0, nil
	case <-ctx.Done():
		bridge.closeAll()
		return 1, ctx.Err()
	}
}

func (a *acpBridge) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{Close: &acp.SessionCloseCapabilities{}},
		},
		AuthMethods: []acp.AuthMethod{},
	}, nil
}

func (a *acpBridge) NewSession(_ context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if len(params.McpServers) > 0 {
		return acp.NewSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": "client-provided MCP servers are not supported; configure MCP servers in the agentfile"})
	}
	if len(params.AdditionalDirectories) > 0 {
		return acp.NewSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": "additionalDirectories are not supported"})
	}
	if !filepath.IsAbs(params.Cwd) {
		return acp.NewSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": "cwd must be an absolute path"})
	}
	info, err := os.Stat(params.Cwd)
	if err != nil {
		return acp.NewSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": fmt.Sprintf("read cwd: %v", err)})
	}
	if !info.IsDir() {
		return acp.NewSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": "cwd is not a directory"})
	}

	id := rand.Text()
	s, err := a.startSession(acp.SessionId(id), params.Cwd)
	if err != nil {
		return acp.NewSessionResponse{}, acp.NewInternalError(map[string]any{"error": err.Error()})
	}
	a.mu.Lock()
	a.sessions[s.id] = s
	a.mu.Unlock()
	return acp.NewSessionResponse{SessionId: s.id}, nil
}

func (a *acpBridge) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	s, ok := a.session(params.SessionId)
	if !ok {
		return acp.PromptResponse{}, acp.NewInvalidParams(map[string]any{"error": "unknown session"})
	}
	content := make([]any, 0, len(params.Prompt))
	for _, block := range params.Prompt {
		switch {
		case block.Text != nil:
			content = append(content, map[string]any{"type": "text", "text": block.Text.Text})
		case block.ResourceLink != nil:
			text := resourceLinkText(block.ResourceLink, s.cwd)
			content = append(content, map[string]any{"type": "text", "text": text})
		default:
			return acp.PromptResponse{}, acp.NewInvalidParams(map[string]any{"error": "only text and resource_link prompt blocks are supported"})
		}
	}
	if !s.beginPrompt() {
		return acp.PromptResponse{}, acp.NewInvalidRequest(map[string]any{"error": "session already has an active prompt"})
	}
	defer s.endPrompt()
	turn := acpTurn{seen: map[int]bool{}, tools: map[int]*claudeTool{}, stop: acp.StopReasonEndTurn}
	if err := s.sendPrompt(&turn, content); err != nil {
		a.dropSession(s)
		return acp.PromptResponse{}, acp.NewInternalError(map[string]any{"error": err.Error()})
	}
	if ctx.Err() != nil {
		_ = s.interrupt()
	}

	for {
		message, err := s.read()
		if err != nil {
			cancelled := s.wasCancelled()
			a.dropSession(s)
			if cancelled {
				return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
			}
			return acp.PromptResponse{}, acp.NewInternalError(map[string]any{"error": err.Error()})
		}
		stop, err := a.translate(s, &turn, message)
		if err != nil {
			a.dropSession(s)
			return acp.PromptResponse{}, acp.NewInternalError(map[string]any{"error": err.Error()})
		}
		if stop != nil {
			return acp.PromptResponse{StopReason: *stop, UserMessageId: params.MessageId}, nil
		}
	}
}

func (a *acpBridge) Cancel(_ context.Context, params acp.CancelNotification) error {
	if s, ok := a.session(params.SessionId); ok {
		return s.interrupt()
	}
	return nil
}

func (a *acpBridge) CloseSession(_ context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	a.mu.Lock()
	s, ok := a.sessions[params.SessionId]
	delete(a.sessions, params.SessionId)
	a.mu.Unlock()
	if !ok {
		return acp.CloseSessionResponse{}, acp.NewInvalidParams(map[string]any{"error": "unknown session"})
	}
	s.close()
	return acp.CloseSessionResponse{}, nil
}

func (a *acpBridge) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, acp.NewMethodNotFound(acp.AgentMethodAuthenticate)
}

func (a *acpBridge) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, acp.NewMethodNotFound(acp.AgentMethodLogout)
}

func (a *acpBridge) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionList)
}

func (a *acpBridge) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionResume)
}

func (a *acpBridge) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)
}

func (a *acpBridge) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetMode)
}

func (a *acpBridge) startSession(id acp.SessionId, cwd string) (*acpSession, error) {
	envs := runEnv(a.options.RuntimeEnvNames, a.options.Env)
	if len(a.options.RuntimeEnvNames) == 0 && a.options.Project != nil {
		envs = runEnv(a.options.Project.AgentFile.Spec.RuntimeEnvNames(), a.options.Env)
	}
	envs["AGENTFILE_RUN_MODE"] = "acp"
	if a.options.Model != "" {
		envs["AGENTFILE_MODEL"] = a.options.Model
	}
	container := "agentfile-acp-" + string(id)
	args := []string{"run", "--rm", "-i", "--name", container}
	args = appendRunEnvironment(args, a.options.EnvFiles, envs)
	args = append(args, "--mount", "type=bind,source="+cwd+",target=/agent/workspace", a.image)
	cmd := exec.CommandContext(a.ctx, a.options.DockerBinary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open container stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open container stdout: %w", err)
	}
	stderr := &prefixBuffer{}
	cmd.Stderr = io.MultiWriter(a.options.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ACP container: %w", err)
	}
	s := &acpSession{
		bridge:    a,
		id:        id,
		container: container,
		cwd:       cwd,
		cmd:       cmd,
		stdin:     stdin,
		scanner:   bufio.NewScanner(stdout),
		stderr:    stderr,
		done:      make(chan error, 1),
	}
	s.scanner.Buffer(make([]byte, 64<<10), harnessMessageLimit)
	go func() { s.done <- cmd.Wait() }()
	if err := s.initialize(); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *acpSession) initialize() error {
	switch s.bridge.harness {
	case "claudecode":
		return s.initializeClaude()
	case "codex":
		return s.initializeCodex()
	case "pi":
		return nil
	default:
		return fmt.Errorf("unsupported ACP harness %s", s.bridge.harness)
	}
}

func (s *acpSession) initializeClaude() error {
	requestID := rand.Text()
	if err := s.write(map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request":    map[string]any{"subtype": "initialize", "hooks": nil},
	}); err != nil {
		return err
	}
	for {
		message, err := s.read()
		if err != nil {
			return err
		}
		if stringValue(message["type"]) != "control_response" {
			continue
		}
		response, _ := message["response"].(map[string]any)
		if stringValue(response["request_id"]) != requestID {
			continue
		}
		if stringValue(response["subtype"]) != "success" {
			return fmt.Errorf("Claude initialization failed: %v", response)
		}
		return nil
	}
}

func (s *acpSession) initializeCodex() error {
	if _, err := s.request("initialize", map[string]any{"clientInfo": map[string]any{
		"name": "agentfile", "title": "Agentfile ACP bridge", "version": "1",
	}}); err != nil {
		return fmt.Errorf("Codex initialization failed: %w", err)
	}
	if err := s.write(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return err
	}
	result, err := s.request("thread/start", map[string]any{
		"cwd": "/agent/workspace", "ephemeral": true,
	})
	if err != nil {
		return fmt.Errorf("Codex thread start failed: %w", err)
	}
	thread := mapValue(result["thread"])
	s.threadID = stringValue(thread["id"])
	if s.threadID == "" {
		return fmt.Errorf("Codex thread start returned no thread id")
	}
	return nil
}

func (s *acpSession) request(method string, params map[string]any) (map[string]any, error) {
	id := rand.Text()
	if err := s.write(map[string]any{"method": method, "id": id, "params": params}); err != nil {
		return nil, err
	}
	for {
		message, err := s.read()
		if err != nil {
			return nil, err
		}
		if fmt.Sprint(message["id"]) != id {
			continue
		}
		if failure := mapValue(message["error"]); failure != nil {
			return nil, fmt.Errorf("%s: %s", fmt.Sprint(failure["code"]), stringValue(failure["message"]))
		}
		return mapValue(message["result"]), nil
	}
}

func (s *acpSession) sendPrompt(turn *acpTurn, content []any) error {
	switch s.bridge.harness {
	case "claudecode":
		return s.write(map[string]any{
			"type":               "user",
			"message":            map[string]any{"role": "user", "content": content},
			"parent_tool_use_id": nil,
			"session_id":         "default",
		})
	case "codex":
		turn.requestID = rand.Text()
		return s.write(map[string]any{
			"method": "turn/start", "id": turn.requestID,
			"params": map[string]any{"threadId": s.threadID, "input": content},
		})
	case "pi":
		turn.requestID = rand.Text()
		return s.write(map[string]any{"id": turn.requestID, "type": "prompt", "message": contentText(content)})
	default:
		return fmt.Errorf("unsupported ACP harness %s", s.bridge.harness)
	}
}

func (a *acpBridge) translate(s *acpSession, turn *acpTurn, message map[string]any) (*acp.StopReason, error) {
	switch s.bridge.harness {
	case "claudecode":
		return a.translateClaude(s, turn, message)
	case "codex":
		return a.translateCodex(s, turn, message)
	case "pi":
		return a.translatePi(s, turn, message)
	default:
		return nil, fmt.Errorf("unsupported ACP harness %s", s.bridge.harness)
	}
}

func (a *acpBridge) translateClaude(s *acpSession, turn *acpTurn, message map[string]any) (*acp.StopReason, error) {
	switch stringValue(message["type"]) {
	case "stream_event":
		event, _ := message["event"].(map[string]any)
		return nil, a.translateStream(s, turn, event)
	case "assistant":
		body, _ := message["message"].(map[string]any)
		content, _ := body["content"].([]any)
		for i, raw := range content {
			if turn.seen[i] {
				continue
			}
			block, _ := raw.(map[string]any)
			switch stringValue(block["type"]) {
			case "text":
				if err := a.update(s.id, acp.UpdateAgentMessageText(stringValue(block["text"]))); err != nil {
					return nil, err
				}
			case "thinking":
				if err := a.update(s.id, acp.UpdateAgentThoughtText(stringValue(block["thinking"]))); err != nil {
					return nil, err
				}
			case "tool_use":
				id, name := stringValue(block["id"]), stringValue(block["name"])
				if err := a.update(s.id, acp.StartToolCall(acp.ToolCallId(id), name,
					acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartRawInput(block["input"]))); err != nil {
					return nil, err
				}
			}
		}
	case "user":
		body, _ := message["message"].(map[string]any)
		content, _ := body["content"].([]any)
		for _, raw := range content {
			block, _ := raw.(map[string]any)
			if stringValue(block["type"]) != "tool_result" {
				continue
			}
			status := acp.ToolCallStatusCompleted
			if value, _ := block["is_error"].(bool); value {
				status = acp.ToolCallStatusFailed
			}
			text := contentText(block["content"])
			opts := []acp.ToolCallUpdateOpt{acp.WithUpdateStatus(status), acp.WithUpdateRawOutput(block["content"])}
			if text != "" {
				opts = append(opts, acp.WithUpdateContent([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(text))}))
			}
			if err := a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(stringValue(block["tool_use_id"])), opts...)); err != nil {
				return nil, err
			}
		}
	case "result":
		if s.wasCancelled() {
			stop := acp.StopReasonCancelled
			return &stop, nil
		}
		stopReason := stringValue(message["stop_reason"])
		subtype := stringValue(message["subtype"])
		switch {
		case subtype == "error_max_turns":
			stop := acp.StopReasonMaxTurnRequests
			return &stop, nil
		case stopReason == "max_tokens":
			stop := acp.StopReasonMaxTokens
			return &stop, nil
		case stopReason == "refusal":
			stop := acp.StopReasonRefusal
			return &stop, nil
		case subtype == "success":
			stop := acp.StopReasonEndTurn
			return &stop, nil
		default:
			return nil, fmt.Errorf("Claude turn failed: %s", claudeResultError(message))
		}
	}
	return nil, nil
}

func (a *acpBridge) translateStream(s *acpSession, turn *acpTurn, event map[string]any) error {
	switch stringValue(event["type"]) {
	case "message_start":
		turn.seen = map[int]bool{}
		turn.tools = map[int]*claudeTool{}
	case "content_block_start":
		index := intValue(event["index"])
		block, _ := event["content_block"].(map[string]any)
		turn.seen[index] = true
		if stringValue(block["type"]) == "tool_use" {
			tool := &claudeTool{id: stringValue(block["id"])}
			turn.tools[index] = tool
			return a.update(s.id, acp.StartToolCall(acp.ToolCallId(tool.id), stringValue(block["name"]),
				acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartRawInput(block["input"])))
		}
	case "content_block_delta":
		index := intValue(event["index"])
		turn.seen[index] = true
		delta, _ := event["delta"].(map[string]any)
		switch stringValue(delta["type"]) {
		case "text_delta":
			return a.update(s.id, acp.UpdateAgentMessageText(stringValue(delta["text"])))
		case "thinking_delta":
			return a.update(s.id, acp.UpdateAgentThoughtText(stringValue(delta["thinking"])))
		case "input_json_delta":
			if tool := turn.tools[index]; tool != nil {
				tool.partial.WriteString(stringValue(delta["partial_json"]))
			}
		}
	case "content_block_stop":
		index := intValue(event["index"])
		tool := turn.tools[index]
		if tool == nil || tool.partial.Len() == 0 {
			return nil
		}
		var input any
		if err := json.Unmarshal([]byte(tool.partial.String()), &input); err != nil {
			return fmt.Errorf("decode Claude tool input: %w", err)
		}
		return a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(tool.id), acp.WithUpdateRawInput(input)))
	}
	return nil
}

func (a *acpBridge) translateCodex(s *acpSession, turn *acpTurn, message map[string]any) (*acp.StopReason, error) {
	if fmt.Sprint(message["id"]) == turn.requestID {
		if failure := mapValue(message["error"]); failure != nil {
			return nil, fmt.Errorf("Codex turn start failed: %s", stringValue(failure["message"]))
		}
		s.setTurnID(stringValue(mapValue(mapValue(message["result"])["turn"])["id"]))
		if turnID := s.currentTurnID(); s.wasCancelled() && turnID != "" {
			_ = s.write(map[string]any{"method": "turn/interrupt", "id": rand.Text(), "params": map[string]any{"threadId": s.threadID, "turnId": turnID}})
		}
		return nil, nil
	}
	method := stringValue(message["method"])
	params := mapValue(message["params"])
	if threadID := stringValue(params["threadId"]); threadID != "" && threadID != s.threadID {
		return nil, nil
	}
	switch method {
	case "turn/started":
		s.setTurnID(stringValue(mapValue(params["turn"])["id"]))
	case "item/agentMessage/delta":
		return nil, a.update(s.id, acp.UpdateAgentMessageText(stringValue(params["delta"])))
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		return nil, a.update(s.id, acp.UpdateAgentThoughtText(stringValue(params["delta"])))
	case "item/started":
		item := mapValue(params["item"])
		id, title, input, ok := codexTool(item)
		if ok {
			return nil, a.update(s.id, acp.StartToolCall(acp.ToolCallId(id), title,
				acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartRawInput(input)))
		}
	case "item/commandExecution/outputDelta", "item/fileChange/outputDelta":
		id, delta := stringValue(params["itemId"]), stringValue(params["delta"])
		if id != "" && delta != "" {
			return nil, a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(id),
				acp.WithUpdateContent([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(delta))})))
		}
	case "item/completed":
		item := mapValue(params["item"])
		id, _, _, ok := codexTool(item)
		if ok {
			status := acp.ToolCallStatusCompleted
			toolStatus := stringValue(item["status"])
			if toolStatus == "failed" || toolStatus == "declined" || intValue(item["exitCode"]) != 0 {
				status = acp.ToolCallStatusFailed
			}
			opts := []acp.ToolCallUpdateOpt{acp.WithUpdateStatus(status), acp.WithUpdateRawOutput(item)}
			if output := codexToolOutput(item); output != "" {
				opts = append(opts, acp.WithUpdateContent([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(output))}))
			}
			return nil, a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(id), opts...))
		}
	case "error":
		if retry, _ := params["willRetry"].(bool); !retry {
			turn.err = stringValue(mapValue(params["error"])["message"])
		}
	case "turn/completed":
		completed := mapValue(params["turn"])
		switch stringValue(completed["status"]) {
		case "completed":
			stop := acp.StopReasonEndTurn
			return &stop, nil
		case "interrupted":
			stop := acp.StopReasonCancelled
			return &stop, nil
		case "failed":
			message := stringValue(mapValue(completed["error"])["message"])
			if message == "" {
				message = turn.err
			}
			return nil, fmt.Errorf("Codex turn failed: %s", message)
		}
	}
	return nil, nil
}

func (a *acpBridge) translatePi(s *acpSession, turn *acpTurn, message map[string]any) (*acp.StopReason, error) {
	if fmt.Sprint(message["id"]) == turn.requestID && stringValue(message["type"]) == "response" {
		if success, _ := message["success"].(bool); !success {
			return nil, fmt.Errorf("Pi prompt failed: %s", stringValue(message["error"]))
		}
		return nil, nil
	}
	switch stringValue(message["type"]) {
	case "message_update":
		event := mapValue(message["assistantMessageEvent"])
		switch stringValue(event["type"]) {
		case "text_delta":
			return nil, a.update(s.id, acp.UpdateAgentMessageText(stringValue(event["delta"])))
		case "thinking_delta":
			return nil, a.update(s.id, acp.UpdateAgentThoughtText(stringValue(event["delta"])))
		case "done":
			if stringValue(event["reason"]) == "length" {
				turn.stop = acp.StopReasonMaxTokens
			}
		case "error":
			if stringValue(event["reason"]) == "aborted" {
				turn.stop = acp.StopReasonCancelled
			} else {
				turn.err = stringValue(event["error"])
				if turn.err == "" {
					turn.err = "model error"
				}
			}
		}
	case "tool_execution_start":
		return nil, a.update(s.id, acp.StartToolCall(acp.ToolCallId(stringValue(message["toolCallId"])), stringValue(message["toolName"]),
			acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartRawInput(message["args"])))
	case "tool_execution_update":
		id, result := stringValue(message["toolCallId"]), message["partialResult"]
		opts := []acp.ToolCallUpdateOpt{acp.WithUpdateRawOutput(result)}
		if output := contentText(mapValue(result)["content"]); output != "" {
			opts = append(opts, acp.WithUpdateContent([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(output))}))
		}
		return nil, a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(id), opts...))
	case "tool_execution_end":
		status := acp.ToolCallStatusCompleted
		if failed, _ := message["isError"].(bool); failed {
			status = acp.ToolCallStatusFailed
		}
		result := message["result"]
		opts := []acp.ToolCallUpdateOpt{acp.WithUpdateStatus(status), acp.WithUpdateRawOutput(result)}
		if output := contentText(mapValue(result)["content"]); output != "" {
			opts = append(opts, acp.WithUpdateContent([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(output))}))
		}
		return nil, a.update(s.id, acp.UpdateToolCall(acp.ToolCallId(stringValue(message["toolCallId"])), opts...))
	case "agent_end":
		// Pi 0.80 emits willRetry but no agent_settled; ACP never queues continuations.
		if retry, _ := message["willRetry"].(bool); retry {
			return nil, nil
		}
		if s.wasCancelled() {
			stop := acp.StopReasonCancelled
			return &stop, nil
		}
		if turn.err != "" {
			return nil, fmt.Errorf("Pi turn failed: %s", turn.err)
		}
		stop := turn.stop
		return &stop, nil
	}
	return nil, nil
}

func codexTool(item map[string]any) (string, string, any, bool) {
	id := stringValue(item["id"])
	switch stringValue(item["type"]) {
	case "commandExecution":
		return id, stringValue(item["command"]), map[string]any{"command": item["command"], "cwd": item["cwd"]}, id != ""
	case "fileChange":
		return id, "Apply file changes", item["changes"], id != ""
	case "mcpToolCall":
		return id, stringValue(item["server"]) + "/" + stringValue(item["tool"]), item["arguments"], id != ""
	case "dynamicToolCall":
		return id, stringValue(item["tool"]), item["arguments"], id != ""
	case "webSearch":
		return id, "Web search: " + stringValue(item["query"]), map[string]any{"query": item["query"]}, id != ""
	}
	return "", "", nil, false
}

func codexToolOutput(item map[string]any) string {
	for _, key := range []string{"aggregatedOutput", "result", "error", "contentItems"} {
		if text := contentText(item[key]); text != "" {
			return text
		}
	}
	return ""
}

func (a *acpBridge) update(sessionID acp.SessionId, update acp.SessionUpdate) error {
	<-a.ready
	return a.conn.SessionUpdate(a.ctx, acp.SessionNotification{SessionId: sessionID, Update: update})
}

func (a *acpBridge) session(id acp.SessionId) (*acpSession, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	return s, ok
}

func (a *acpBridge) closeAll() {
	a.mu.Lock()
	sessions := a.sessions
	a.sessions = map[acp.SessionId]*acpSession{}
	a.mu.Unlock()
	for _, s := range sessions {
		s.close()
	}
}

func (a *acpBridge) dropSession(s *acpSession) {
	a.mu.Lock()
	if a.sessions[s.id] == s {
		delete(a.sessions, s.id)
	}
	a.mu.Unlock()
	s.close()
}

func (s *acpSession) beginPrompt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return false
	}
	s.active = true
	s.cancel = false
	s.turnID = ""
	return true
}

func (s *acpSession) endPrompt() {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

func (s *acpSession) wasCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel
}

func (s *acpSession) setTurnID(id string) {
	s.mu.Lock()
	s.turnID = id
	s.mu.Unlock()
}

func (s *acpSession) currentTurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnID
}

func (s *acpSession) interrupt() error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	s.cancel = true
	s.mu.Unlock()
	switch s.bridge.harness {
	case "claudecode":
		return s.write(map[string]any{
			"type":       "control_request",
			"request_id": rand.Text(),
			"request":    map[string]any{"subtype": "interrupt"},
		})
	case "codex":
		turnID := s.currentTurnID()
		if turnID == "" {
			return nil
		}
		return s.write(map[string]any{"method": "turn/interrupt", "id": rand.Text(), "params": map[string]any{
			"threadId": s.threadID, "turnId": turnID,
		}})
	case "pi":
		return s.write(map[string]any{"id": rand.Text(), "type": "abort"})
	default:
		return nil
	}
}

func (s *acpSession) write(value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := json.NewEncoder(s.stdin).Encode(value); err != nil {
		return fmt.Errorf("write %s stream: %w", s.bridge.harness, err)
	}
	return nil
}

func (s *acpSession) read() (map[string]any, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read %s stream: %w", s.bridge.harness, err)
		}
		err := <-s.done
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 64 && strings.Contains(s.stderr.String(), "agentfile: unsupported run mode acp") {
			return nil, fmt.Errorf("image %q does not support ACP; rebuild it with a current af", s.bridge.image)
		}
		if err != nil {
			if stderr := strings.TrimSpace(s.stderr.String()); stderr != "" {
				return nil, fmt.Errorf("ACP container exited: %w: %s", err, stderr)
			}
			return nil, fmt.Errorf("ACP container exited: %w", err)
		}
		return nil, fmt.Errorf("ACP container exited")
	}
	var message map[string]any
	if err := json.Unmarshal(s.scanner.Bytes(), &message); err != nil {
		return nil, fmt.Errorf("decode %s stream: %w", s.bridge.harness, err)
	}
	return message, nil
}

func resourceLinkText(link *acp.ContentBlockResourceLink, cwd string) string {
	reference := link.Uri
	if uri, err := url.Parse(link.Uri); err == nil && uri.Scheme == "file" && (uri.Host == "" || uri.Host == "localhost") {
		if relative, err := filepath.Rel(cwd, uri.Path); err == nil && filepath.IsLocal(relative) {
			reference = filepath.ToSlash(filepath.Join("/agent/workspace", relative))
		}
	}
	return fmt.Sprintf("The user referenced resource %q at %s.", link.Name, reference)
}

func (s *acpSession) close() {
	s.stop.Do(func() {
		_ = s.interrupt()
		_ = s.stdin.Close()
		remove := exec.Command(s.bridge.options.DockerBinary, "rm", "-f", s.container)
		remove.Stdout = io.Discard
		remove.Stderr = s.bridge.options.Stderr
		if err := remove.Run(); err != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
}

func stringValue(value any) string {
	stringValue, _ := value.(string)
	return stringValue
}

func intValue(value any) int {
	number, _ := value.(float64)
	return int(number)
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func contentText(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case []any:
		var text strings.Builder
		for _, item := range value {
			text.WriteString(contentText(item))
		}
		return text.String()
	case map[string]any:
		for _, key := range []string{"text", "message", "content"} {
			if text := contentText(value[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func claudeResultError(message map[string]any) string {
	if result := stringValue(message["result"]); result != "" {
		return result
	}
	if values, ok := message["errors"].([]any); ok {
		parts := make([]string, 0, len(values))
		for _, value := range values {
			parts = append(parts, fmt.Sprint(value))
		}
		return strings.Join(parts, "; ")
	}
	return stringValue(message["subtype"])
}
