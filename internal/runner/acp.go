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

const claudeMessageLimit = 16 << 20

type acpBridge struct {
	ctx     context.Context
	options Options
	image   string
	conn    *acp.AgentSideConnection
	ready   chan struct{}

	mu       sync.Mutex
	sessions map[acp.SessionId]*claudeSession
}

type claudeSession struct {
	bridge    *acpBridge
	id        acp.SessionId
	container string
	cwd       string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	scanner   *bufio.Scanner
	stderr    *prefixBuffer
	done      chan error

	writeMu sync.Mutex
	mu      sync.Mutex
	active  bool
	cancel  bool
	stop    sync.Once
}

type claudeTurn struct {
	seen  map[int]bool
	tools map[int]*claudeTool
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
	if harness != "claudecode" {
		return 1, fmt.Errorf("--acp currently supports only Claude Code; image uses %s", harness)
	}
	image, err := resolveRunImage(ctx, options)
	if err != nil {
		return 1, err
	}
	bridge := &acpBridge{
		ctx:      ctx,
		options:  options,
		image:    image,
		ready:    make(chan struct{}),
		sessions: map[acp.SessionId]*claudeSession{},
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
	content := make([]map[string]any, 0, len(params.Prompt))
	for _, block := range params.Prompt {
		switch {
		case block.Text != nil:
			content = append(content, map[string]any{"type": "text", "text": block.Text.Text})
		case block.ResourceLink != nil:
			content = append(content, map[string]any{"type": "text", "text": resourceLinkText(block.ResourceLink, s.cwd)})
		default:
			return acp.PromptResponse{}, acp.NewInvalidParams(map[string]any{"error": "only text and resource_link prompt blocks are supported"})
		}
	}
	if !s.beginPrompt() {
		return acp.PromptResponse{}, acp.NewInvalidRequest(map[string]any{"error": "session already has an active prompt"})
	}
	defer s.endPrompt()
	if err := s.write(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": content},
		"parent_tool_use_id": nil,
		"session_id":         "default",
	}); err != nil {
		a.dropSession(s)
		return acp.PromptResponse{}, acp.NewInternalError(map[string]any{"error": err.Error()})
	}
	if ctx.Err() != nil {
		_ = s.interrupt()
	}

	turn := claudeTurn{seen: map[int]bool{}, tools: map[int]*claudeTool{}}
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

func (a *acpBridge) startSession(id acp.SessionId, cwd string) (*claudeSession, error) {
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
	s := &claudeSession{
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
	s.scanner.Buffer(make([]byte, 64<<10), claudeMessageLimit)
	go func() { s.done <- cmd.Wait() }()
	if err := s.initialize(); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *claudeSession) initialize() error {
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

func (a *acpBridge) translate(s *claudeSession, turn *claudeTurn, message map[string]any) (*acp.StopReason, error) {
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
			text := claudeContentText(block["content"])
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

func (a *acpBridge) translateStream(s *claudeSession, turn *claudeTurn, event map[string]any) error {
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

func (a *acpBridge) update(sessionID acp.SessionId, update acp.SessionUpdate) error {
	<-a.ready
	return a.conn.SessionUpdate(a.ctx, acp.SessionNotification{SessionId: sessionID, Update: update})
}

func (a *acpBridge) session(id acp.SessionId) (*claudeSession, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	return s, ok
}

func (a *acpBridge) closeAll() {
	a.mu.Lock()
	sessions := a.sessions
	a.sessions = map[acp.SessionId]*claudeSession{}
	a.mu.Unlock()
	for _, s := range sessions {
		s.close()
	}
}

func (a *acpBridge) dropSession(s *claudeSession) {
	a.mu.Lock()
	if a.sessions[s.id] == s {
		delete(a.sessions, s.id)
	}
	a.mu.Unlock()
	s.close()
}

func (s *claudeSession) beginPrompt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return false
	}
	s.active = true
	s.cancel = false
	return true
}

func (s *claudeSession) endPrompt() {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

func (s *claudeSession) wasCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel
}

func (s *claudeSession) interrupt() error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	s.cancel = true
	s.mu.Unlock()
	return s.write(map[string]any{
		"type":       "control_request",
		"request_id": rand.Text(),
		"request":    map[string]any{"subtype": "interrupt"},
	})
}

func (s *claudeSession) write(value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := json.NewEncoder(s.stdin).Encode(value); err != nil {
		return fmt.Errorf("write Claude stream: %w", err)
	}
	return nil
}

func (s *claudeSession) read() (map[string]any, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read Claude stream: %w", err)
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
		return nil, fmt.Errorf("decode Claude stream: %w", err)
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

func (s *claudeSession) close() {
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

func claudeContentText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	blocks, _ := value.([]any)
	var text strings.Builder
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		if stringValue(block["type"]) == "text" {
			text.WriteString(stringValue(block["text"]))
		}
	}
	return text.String()
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
