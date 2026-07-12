package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/itaysk/agentfile/internal/agentfile"
)

type acpTestProcess struct {
	input    *io.PipeWriter
	messages chan map[string]any
	done     chan error
}

func TestRunACPStreamsClaudeSession(t *testing.T) {
	docker, logPath := installFakeACPDocker(t)
	t.Setenv("ACP_TOKEN", "secret")
	inputLog := filepath.Join(t.TempDir(), "claude-input.log")
	t.Setenv("ACP_INPUT_LOG", inputLog)
	process := startACPTestProcess(t, Options{
		Image:           "acme/claude:latest",
		Harness:         "claudecode",
		RuntimeEnvNames: []string{"ACP_TOKEN"},
		DockerBinary:    docker,
		Env:             map[string]string{"EXPLICIT": "value"},
		Model:           "claude-test",
		Stderr:          io.Discard,
	})

	process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
	initialize := process.response(t, 1)
	result := initialize["result"].(map[string]any)
	capabilities := result["agentCapabilities"].(map[string]any)
	sessionCapabilities := capabilities["sessionCapabilities"].(map[string]any)
	if _, ok := sessionCapabilities["close"]; !ok {
		t.Fatalf("initialize result = %#v, want session close capability", result)
	}

	workspace := filepath.Join(t.TempDir(), "with:colon")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	resource := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(resource, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	process.send(t, 2, "session/new", map[string]any{"cwd": workspace, "mcpServers": []any{}})
	newSession := process.response(t, 2)
	sessionID := newSession["result"].(map[string]any)["sessionId"].(string)
	process.send(t, 3, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt": []any{
			map[string]any{"type": "text", "text": "hello"},
			map[string]any{"type": "resource_link", "uri": "file://" + filepath.ToSlash(resource), "name": "README.md"},
		},
	})

	var textChunks, toolStarts, completed int
	for {
		message := process.receive(t)
		if message["id"] == float64(3) {
			if got := message["result"].(map[string]any)["stopReason"]; got != "end_turn" {
				t.Fatalf("prompt stopReason = %v, want end_turn", got)
			}
			break
		}
		params := message["params"].(map[string]any)
		update := params["update"].(map[string]any)
		switch update["sessionUpdate"] {
		case "agent_message_chunk":
			textChunks++
		case "tool_call":
			toolStarts++
		case "tool_call_update":
			if update["status"] == "completed" {
				completed++
			}
		}
	}
	if textChunks != 1 || toolStarts != 1 || completed != 1 {
		t.Fatalf("updates = text:%d tool:%d completed:%d, want 1 each", textChunks, toolStarts, completed)
	}

	process.send(t, 4, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []any{map[string]any{"type": "text", "text": "follow up"}},
	})
	if got := process.response(t, 4)["result"].(map[string]any)["stopReason"]; got != "end_turn" {
		t.Fatalf("follow-up stopReason = %v, want end_turn", got)
	}
	process.send(t, 5, "session/close", map[string]any{"sessionId": sessionID})
	process.response(t, 5)
	process.close(t)

	log := dockerLog(t, logPath)
	for _, want := range []string{
		"run --rm -i --name agentfile-acp-",
		"-e AGENTFILE_RUN_MODE=acp",
		"-e ACP_TOKEN=secret",
		"-e EXPLICIT=value",
		"-e AGENTFILE_MODEL=claude-test",
		"--mount type=bind,source=" + workspace + ",target=/agent/workspace",
		"acme/claude:latest",
		"rm -f agentfile-acp-",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker log does not contain %q:\n%s", want, log)
		}
	}
	input, err := os.ReadFile(inputLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(input), `The user referenced resource \"README.md\" at /agent/workspace/README.md.`) {
		t.Fatalf("Claude input does not contain translated resource link:\n%s", input)
	}
}

func TestRunACPStreamsCodexAndPiSessions(t *testing.T) {
	for _, harness := range []string{"codex", "pi"} {
		t.Run(harness, func(t *testing.T) {
			docker, logPath := installFakeACPDocker(t)
			inputLog := filepath.Join(t.TempDir(), harness+"-input.log")
			t.Setenv("ACP_INPUT_LOG", inputLog)
			process := startACPTestProcess(t, Options{
				Image:        "acme/" + harness + ":latest",
				Harness:      harness,
				DockerBinary: docker,
				Stderr:       io.Discard,
			})
			process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
			process.response(t, 1)
			process.send(t, 2, "session/new", map[string]any{"cwd": t.TempDir(), "mcpServers": []any{}})
			sessionID := process.response(t, 2)["result"].(map[string]any)["sessionId"].(string)
			process.send(t, 3, "session/prompt", map[string]any{
				"sessionId": sessionID,
				"prompt":    []any{map[string]any{"type": "text", "text": "hello"}},
			})

			var messages, thoughts, tools, completed int
			for {
				message := process.receive(t)
				if message["id"] == float64(3) {
					if got := message["result"].(map[string]any)["stopReason"]; got != "end_turn" {
						t.Fatalf("prompt stopReason = %v, want end_turn", got)
					}
					break
				}
				update := message["params"].(map[string]any)["update"].(map[string]any)
				switch update["sessionUpdate"] {
				case "agent_message_chunk":
					messages++
				case "agent_thought_chunk":
					thoughts++
				case "tool_call":
					tools++
				case "tool_call_update":
					if update["status"] == "completed" {
						completed++
					}
				}
			}
			if messages != 1 || thoughts != 1 || tools != 1 || completed != 1 {
				t.Fatalf("updates = messages:%d thoughts:%d tools:%d completed:%d, want 1 each", messages, thoughts, tools, completed)
			}
			process.send(t, 4, "session/prompt", map[string]any{
				"sessionId": sessionID,
				"prompt":    []any{map[string]any{"type": "text", "text": "follow up"}},
			})
			if got := process.response(t, 4)["result"].(map[string]any)["stopReason"]; got != "end_turn" {
				t.Fatalf("follow-up stopReason = %v, want end_turn", got)
			}
			process.send(t, 5, "session/close", map[string]any{"sessionId": sessionID})
			process.response(t, 5)
			process.close(t)
			if !strings.Contains(dockerLog(t, logPath), "acme/"+harness+":latest") {
				t.Fatalf("docker log does not contain harness image:\n%s", dockerLog(t, logPath))
			}
			input, err := os.ReadFile(inputLog)
			if err != nil || !strings.Contains(string(input), "hello") {
				t.Fatalf("%s input = %q, %v; want prompt", harness, input, err)
			}
		})
	}
}

func TestRunACPCancelsWithoutStoppingSession(t *testing.T) {
	for _, tt := range []struct{ harness, image string }{
		{"claudecode", "acme/claude:latest"},
		{"codex", "acme/codex:latest"},
		{"pi", "acme/pi:latest"},
	} {
		t.Run(tt.harness, func(t *testing.T) {
			docker, _ := installFakeACPDocker(t)
			t.Setenv("ACP_WAIT_INTERRUPT", "1")
			userSeen := filepath.Join(t.TempDir(), "user-seen")
			t.Setenv("ACP_USER_SEEN_FILE", userSeen)
			process := startACPTestProcess(t, Options{Image: tt.image, Harness: tt.harness, DockerBinary: docker, Stderr: io.Discard})
			process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
			process.response(t, 1)
			process.send(t, 2, "session/new", map[string]any{"cwd": t.TempDir(), "mcpServers": []any{}})
			sessionID := process.response(t, 2)["result"].(map[string]any)["sessionId"].(string)
			process.send(t, 3, "session/prompt", map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "wait"}}})
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(userSeen); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("timed out waiting for %s prompt input", tt.harness)
				}
				time.Sleep(time.Millisecond)
			}
			process.notify(t, "session/cancel", map[string]any{"sessionId": sessionID})
			if got := process.response(t, 3)["result"].(map[string]any)["stopReason"]; got != "cancelled" {
				t.Fatalf("prompt stopReason = %v, want cancelled", got)
			}
			process.send(t, 4, "session/close", map[string]any{"sessionId": sessionID})
			process.response(t, 4)
			process.close(t)
		})
	}
}

func TestRunACPBuildsPromptlessClaude(t *testing.T) {
	docker, logPath := installFakeACPDocker(t)
	project := runnerTestProject(t)
	project.AgentFile.Spec.Harness = agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}
	project.AgentFile.Spec.LLM = agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-test"}}
	project.AgentFile.Spec.Prompt = nil
	process := startACPTestProcess(t, Options{Project: project, DockerBinary: docker, Stderr: io.Discard})
	process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
	process.response(t, 1)
	process.close(t)
	if !strings.Contains(dockerLog(t, logPath), "build -t test-agent:latest") {
		t.Fatalf("docker log does not contain promptless project build:\n%s", dockerLog(t, logPath))
	}
}

func TestRunACPOldImageGetsRebuildError(t *testing.T) {
	docker, _ := installFakeACPDocker(t)
	t.Setenv("ACP_OLD_IMAGE", "1")
	process := startACPTestProcess(t, Options{Image: "acme/old:latest", Harness: "claudecode", DockerBinary: docker, Stderr: io.Discard})
	process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
	process.response(t, 1)
	process.send(t, 2, "session/new", map[string]any{"cwd": t.TempDir(), "mcpServers": []any{}})
	response := process.response(t, 2)
	errorObject := response["error"].(map[string]any)
	data := errorObject["data"].(map[string]any)
	if !strings.Contains(fmt.Sprint(data["error"]), "rebuild it with a current af") {
		t.Fatalf("session/new error = %#v, want rebuild guidance", errorObject)
	}
	process.close(t)
}

func TestRunACPDoesNotMisdiagnoseUsageErrorAsOldImage(t *testing.T) {
	docker, _ := installFakeACPDocker(t)
	t.Setenv("ACP_USAGE_ERROR", "1")
	process := startACPTestProcess(t, Options{Image: "acme/current:latest", Harness: "claudecode", DockerBinary: docker, Stderr: io.Discard})
	process.send(t, 1, "initialize", map[string]any{"protocolVersion": 1})
	process.response(t, 1)
	process.send(t, 2, "session/new", map[string]any{"cwd": t.TempDir(), "mcpServers": []any{}})
	response := process.response(t, 2)
	errorObject := response["error"].(map[string]any)
	data := errorObject["data"].(map[string]any)
	message := fmt.Sprint(data["error"])
	if !strings.Contains(message, "exit status 64") || !strings.Contains(message, "ACP_TOKEN must not contain newlines") || strings.Contains(message, "rebuild") {
		t.Fatalf("session/new error = %q, want the usage error without rebuild guidance", message)
	}
	process.close(t)
}

func TestRunACPRejectsUnknownHarnessAndOverrides(t *testing.T) {
	prompt := "no"
	for _, tt := range []struct {
		name    string
		options Options
		want    string
	}{
		{name: "unknown", options: Options{Image: "unknown", Harness: "unknown"}, want: "does not support harness"},
		{name: "legacy", options: Options{Image: "legacy"}, want: "missing build.agentfile.harness"},
		{name: "prompt", options: Options{Image: "claude", Harness: "claudecode", Prompt: &prompt}, want: "--prompt cannot"},
		{name: "workspace", options: Options{Image: "claude", Harness: "claudecode", Workspace: "/tmp"}, want: "--workspace cannot"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.options.Mode = RunModeACP
			code, err := Run(context.Background(), tt.options)
			if code != 1 || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run = (%d, %v), want error containing %q", code, err, tt.want)
			}
		})
	}
}

func TestACPNewSessionRejectsClientConfiguration(t *testing.T) {
	bridge := &acpBridge{}
	if _, err := bridge.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir(), McpServers: []acp.McpServer{{}}}); err == nil || !strings.Contains(err.Error(), "client-provided MCP") {
		t.Fatalf("NewSession MCP error = %v, want client-provided MCP rejection", err)
	}
	if _, err := bridge.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir(), McpServers: []acp.McpServer{}, AdditionalDirectories: []string{t.TempDir()}}); err == nil || !strings.Contains(err.Error(), "additionalDirectories") {
		t.Fatalf("NewSession additionalDirectories error = %v, want rejection", err)
	}
}

func startACPTestProcess(t *testing.T, options Options) *acpTestProcess {
	t.Helper()
	options.Mode = RunModeACP
	agentInput, clientInput := io.Pipe()
	clientOutput, agentOutput := io.Pipe()
	options.Stdin = agentInput
	options.Stdout = agentOutput
	done := make(chan error, 1)
	go func() {
		code, err := Run(context.Background(), options)
		_ = agentOutput.Close()
		if err == nil && code != 0 {
			err = fmt.Errorf("exit code %d", code)
		}
		done <- err
	}()
	messages := make(chan map[string]any, 32)
	go func() {
		scanner := bufio.NewScanner(clientOutput)
		for scanner.Scan() {
			var message map[string]any
			if json.Unmarshal(scanner.Bytes(), &message) == nil {
				messages <- message
			}
		}
		close(messages)
	}()
	return &acpTestProcess{input: clientInput, messages: messages, done: done}
}

func (p *acpTestProcess) send(t *testing.T, id int, method string, params any) {
	t.Helper()
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func (p *acpTestProcess) notify(t *testing.T, method string, params any) {
	t.Helper()
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (p *acpTestProcess) write(t *testing.T, message any) {
	t.Helper()
	if err := json.NewEncoder(p.input).Encode(message); err != nil {
		t.Fatal(err)
	}
}

func (p *acpTestProcess) response(t *testing.T, id int) map[string]any {
	t.Helper()
	for {
		message := p.receive(t)
		if message["id"] == float64(id) {
			return message
		}
	}
}

func (p *acpTestProcess) receive(t *testing.T) map[string]any {
	t.Helper()
	select {
	case message, ok := <-p.messages:
		if !ok {
			t.Fatal("ACP output closed")
		}
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ACP message")
		return nil
	}
}

func (p *acpTestProcess) close(t *testing.T) {
	t.Helper()
	_ = p.input.Close()
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("Run = %v, want success", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to exit")
	}
}

func installFakeACPDocker(t *testing.T) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	dockerPath := filepath.Join(binDir, "docker")
	writeRunnerTestFile(t, dockerPath, `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"
if [ "$1" = "rm" ]; then exit 0; fi
if [ "$1" != "run" ]; then exit 0; fi
if [ "${ACP_OLD_IMAGE:-}" = 1 ]; then
  echo 'agentfile: unsupported run mode acp' >&2
  exit 64
fi
if [ "${ACP_USAGE_ERROR:-}" = 1 ]; then
  echo 'agentfile: ACP_TOKEN must not contain newlines' >&2
  exit 64
fi
case "$*" in
  *codex*)
    while IFS= read -r line; do
      if [ -n "${ACP_INPUT_LOG:-}" ]; then printf '%s\n' "$line" >> "$ACP_INPUT_LOG"; fi
      id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
      case "$line" in
        *'"method":"initialize"'*)
          printf '{"id":"%s","result":{"userAgent":"test"}}\n' "$id"
          ;;
        *'"method":"thread/start"'*)
          printf '{"id":"%s","result":{"thread":{"id":"thread-1"}}}\n' "$id"
          ;;
        *'"method":"turn/start"'*)
          printf '{"id":"%s","result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}\n' "$id"
          printf '%s\n' '{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1"}}}'
          if [ -n "${ACP_USER_SEEN_FILE:-}" ]; then : > "$ACP_USER_SEEN_FILE"; fi
          if [ -n "${ACP_WAIT_INTERRUPT:-}" ]; then continue; fi
          printf '%s\n' '{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"reason-1","summaryIndex":0,"delta":"Thinking"}}'
          printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"message-1","delta":"Hi"}}'
          printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","startedAtMs":1,"item":{"id":"tool-1","type":"commandExecution","command":"pwd","cwd":"/agent/workspace","commandActions":[],"status":"inProgress"}}}'
          printf '%s\n' '{"method":"item/commandExecution/outputDelta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"tool-1","delta":"/agent/workspace\n"}}'
          printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","completedAtMs":2,"item":{"id":"tool-1","type":"commandExecution","command":"pwd","cwd":"/agent/workspace","commandActions":[],"status":"completed","aggregatedOutput":"/agent/workspace\n","exitCode":0}}}'
          printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}'
          ;;
        *'"method":"turn/interrupt"'*)
          printf '{"id":"%s","result":{}}\n' "$id"
          printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}'
          ;;
      esac
    done
    exit 0
    ;;
  *'/pi:'*)
    while IFS= read -r line; do
      if [ -n "${ACP_INPUT_LOG:-}" ]; then printf '%s\n' "$line" >> "$ACP_INPUT_LOG"; fi
      id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
      case "$line" in
        *'"type":"prompt"'*)
          printf '{"id":"%s","type":"response","command":"prompt","success":true}\n' "$id"
          if [ -n "${ACP_USER_SEEN_FILE:-}" ]; then : > "$ACP_USER_SEEN_FILE"; fi
          if [ -n "${ACP_WAIT_INTERRUPT:-}" ]; then continue; fi
          printf '%s\n' '{"type":"agent_start"}'
          printf '%s\n' '{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"Thinking"}}'
          printf '%s\n' '{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"Hi"}}'
          printf '%s\n' '{"type":"tool_execution_start","toolCallId":"tool-1","toolName":"read","args":{"path":"README.md"}}'
          printf '%s\n' '{"type":"tool_execution_update","toolCallId":"tool-1","toolName":"read","partialResult":{"content":[{"type":"text","text":"partial"}]}}'
          printf '%s\n' '{"type":"tool_execution_end","toolCallId":"tool-1","toolName":"read","result":{"content":[{"type":"text","text":"contents"}]},"isError":false}'
          printf '%s\n' '{"type":"agent_end","messages":[],"willRetry":false}'
          ;;
        *'"type":"abort"'*)
          printf '{"id":"%s","type":"response","command":"abort","success":true}\n' "$id"
          printf '%s\n' '{"type":"agent_end","messages":[],"willRetry":false}'
          ;;
      esac
    done
    exit 0
    ;;
esac
while IFS= read -r line; do
  if [ -n "${ACP_INPUT_LOG:-}" ]; then printf '%s\n' "$line" >> "$ACP_INPUT_LOG"; fi
  case "$line" in
    *'"subtype":"initialize"'*)
      request_id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s","response":{}}}\n' "$request_id"
      ;;
    *'"subtype":"interrupt"'*)
      request_id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
      printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s","response":{}}}\n' "$request_id"
      printf '%s\n' '{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["interrupted"]}'
      ;;
    *'"type":"user"'*)
      if [ -n "${ACP_USER_SEEN_FILE:-}" ]; then : > "$ACP_USER_SEEN_FILE"; fi
      if [ -n "${ACP_WAIT_INTERRUPT:-}" ]; then continue; fi
      printf '%s\n' '{"type":"system","subtype":"init","session_id":"claude-session"}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"message_start"}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_stop","index":0}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool-1","name":"Read","input":{}}}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"README.md\"}"}}}'
      printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_stop","index":1}}'
      printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"Hi"},{"type":"tool_use","id":"tool-1","name":"Read","input":{"path":"README.md"}}]}}'
      printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":"contents","is_error":false}]}}'
      printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn"}'
      ;;
  esac
done
`)
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_ARGS_LOG", logPath)
	return dockerPath, logPath
}
