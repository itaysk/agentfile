package build

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func TestStageContextWritesCodexRuntimeLayout(t *testing.T) {
	contextDir := t.TempDir()
	skillDir := t.TempDir()
	writeBuildTestFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: helper\n---\nbody\n")
	version := agentfile.DefaultVersion
	envValue := "info"
	project := &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "codex-agent",
				Version: &version,
			},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5-mini"}},
				Envs:    []agentfile.Env{{Name: "LOG_LEVEL", Value: &envValue}},
				MCPs: []agentfile.MCP{
					{
						Name: "time",
						Stdio: &agentfile.StdioMCP{
							Command: []string{"uv", "tool", "run", "mcp-server-time"},
							Envs:    []agentfile.Env{{Name: "EXAMPLE", Value: &envValue}},
						},
					},
				},
			},
		},
	}
	assets := &agentfile.ResolvedAssets{
		Prompt:          "say hi",
		HasPrompt:       true,
		SystemPrompt:    "be concise",
		HasSystemPrompt: true,
		Skills:          []agentfile.ResolvedSkill{{Name: "helper", Dir: skillDir}},
	}

	if err := StageContext(contextDir, project, assets); err != nil {
		t.Fatalf("StageContext returned error: %v", err)
	}

	assertFileContains(t, filepath.Join(contextDir, "agentfile", "codex", "home", ".codex", "config.toml"), "project_doc_max_bytes = 0")
	assertFileContains(t, filepath.Join(contextDir, "agentfile", "codex", "home", ".codex", "config.toml"), `model_instructions_file = "/agent/agentfile/system-prompt.md"`)
	assertFileContains(t, filepath.Join(contextDir, "agentfile", "codex", "home", ".codex", "config.toml"), `[mcp_servers."time"]`)
	assertPathExists(t, filepath.Join(contextDir, "agentfile", "skills", "helper", "SKILL.md"))
	assertPathExists(t, filepath.Join(contextDir, "agentfile", "codex", "home", ".agents", "skills", "helper", "SKILL.md"))
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), `CODEX_API_KEY="$OPENAI_API_KEY"`)
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), `if [ -z "${LOG_LEVEL+x}" ]; then export LOG_LEVEL='info'; fi`)
}

func TestStageContextWritesClaudeMCPAndBareMode(t *testing.T) {
	contextDir := t.TempDir()
	version := agentfile.DefaultVersion
	headerValue := "Bearer token"
	project := &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "claude-agent",
				Version: &version,
			},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{ClaudeCode: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-haiku-4-5"}},
				MCPs: []agentfile.MCP{
					{
						Name: "search",
						HTTP: &agentfile.HTTPMCP{
							URL:     "https://example.com/mcp",
							Headers: []agentfile.Header{{Name: "Authorization", Value: &headerValue}},
						},
					},
				},
			},
		},
	}
	assets := &agentfile.ResolvedAssets{Prompt: "say hi", HasPrompt: true}

	if err := StageContext(contextDir, project, assets); err != nil {
		t.Fatalf("StageContext returned error: %v", err)
	}

	var mcpConfig struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	data, err := os.ReadFile(filepath.Join(contextDir, "agentfile", "claudecode", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		t.Fatal(err)
	}
	if got := mcpConfig.MCPServers["search"].Headers["Authorization"]; got != "Bearer token" {
		t.Fatalf("header = %q, want Bearer token", got)
	}
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), "--bare")
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), "--mcp-config /agent/agentfile/claudecode/mcp.json")
}

func TestStageContextWritesPiRuntimeLayout(t *testing.T) {
	contextDir := t.TempDir()
	version := agentfile.DefaultVersion
	project := &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "pi-agent",
				Version: &version,
			},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Pi: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5-mini"}},
			},
		},
	}
	assets := &agentfile.ResolvedAssets{Prompt: "say hi", HasPrompt: true}

	if err := StageContext(contextDir, project, assets); err != nil {
		t.Fatalf("StageContext returned error: %v", err)
	}

	assertPathExists(t, filepath.Join(contextDir, "agentfile", "pi", "home"))
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), "PI_CODING_AGENT_DIR=/agent/agentfile/pi/home")
	assertFileContains(t, filepath.Join(contextDir, "entrypoint"), `--provider "$AGENTFILE_PROVIDER"`)
}

func TestShellQuotePreservesSingleQuotesAndTrailingNewlines(t *testing.T) {
	value := "say 'hi'\n\n"
	script := "value=" + shQuote(value) + "\nprintf '%s' \"$value\""
	output, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != value {
		t.Fatalf("quoted shell value = %q, want %q", string(output), value)
	}
}

func writeBuildTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q\ncontent:\n%s", path, want, string(data))
	}
}
