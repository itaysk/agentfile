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

func TestConfigRefNames(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "none", content: `{"a": "b"}`, want: ""},
		{name: "deduped and sorted", content: `"__AGENTFILE_REF_B__","__AGENTFILE_REF_A__","__AGENTFILE_REF_B__"`, want: "A,B"},
		{name: "underscored name", content: `"__AGENTFILE_REF_A__B___"`, want: "A__B_"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(configRefNames(tt.content), ","); got != tt.want {
				t.Fatalf("configRefNames(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestWriteRuntimeRenderExecutesByteForByte(t *testing.T) {
	secret := `Bearer x"y\z'w$v,u&t` + "`s"
	static := `lit$eral` + "`bs\\\"q'"
	outDir := t.TempDir()

	mcps := []agentfile.MCP{
		{
			Name: "search",
			HTTP: &agentfile.HTTPMCP{
				URL: "https://example.com/mcp",
				Headers: []agentfile.Header{
					{Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "SEARCH_MCP_AUTH"}}},
					{Name: "X-Static", ValueSource: agentfile.ValueSource{Value: &static}},
				},
			},
		},
	}
	data, err := json.MarshalIndent(map[string]any{"mcpServers": claudeMCPServers(mcps)}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	jsonContent := string(data) + "\n"
	var tomlBuilder strings.Builder
	writeCodexMCPConfig(&tomlBuilder, mcps)
	tomlContent := tomlBuilder.String()

	writeBuildTestFile(t, filepath.Join(outDir, "mcp.json"), jsonContent)
	writeBuildTestFile(t, filepath.Join(outDir, "config.toml"), tomlContent)
	var script strings.Builder
	script.WriteString("set -eu\n")
	writeRuntimeRender(&script, []string{"SEARCH_MCP_AUTH"}, []configFile{
		{path: filepath.Join(outDir, "mcp.json"), content: jsonContent},
		{path: filepath.Join(outDir, "config.toml"), content: tomlContent},
	})
	cmd := exec.Command("sh", "-c", script.String())
	cmd.Env = append(os.Environ(), "SEARCH_MCP_AUTH="+secret)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render script failed: %v\n%s\nscript:\n%s", err, output, script.String())
	}

	var mcpConfig struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	rendered, err := os.ReadFile(filepath.Join(outDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rendered, &mcpConfig); err != nil {
		t.Fatalf("rendered mcp.json is invalid JSON: %v\n%s", err, rendered)
	}
	if got := mcpConfig.MCPServers["search"].Headers["Authorization"]; got != secret {
		t.Fatalf("rendered Authorization = %q, want %q", got, secret)
	}
	if got := mcpConfig.MCPServers["search"].Headers["X-Static"]; got != static {
		t.Fatalf("rendered X-Static = %q, want %q", got, static)
	}
	renderedTOML, err := os.ReadFile(filepath.Join(outDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	escapedSecret := `Bearer x\"y\\z'w$v,u&t` + "`s"
	wantTOML := strings.ReplaceAll(tomlContent, refToken("SEARCH_MCP_AUTH"), escapedSecret)
	if string(renderedTOML) != wantTOML {
		t.Fatalf("rendered config.toml = %q, want %q", renderedTOML, wantTOML)
	}
}

func TestWriteRuntimeRenderRejectsNewlines(t *testing.T) {
	var script strings.Builder
	script.WriteString("set -eu\n")
	writeRuntimeRender(&script, []string{"TOKEN"}, nil)
	cmd := exec.Command("sh", "-c", script.String())
	cmd.Env = append(os.Environ(), "TOKEN=line1\nline2")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("render script succeeded with newline value:\n%s", output)
	}
	if !strings.Contains(string(output), "TOKEN must not contain newlines") {
		t.Fatalf("output = %q, want newline rejection message", output)
	}
}

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
				Envs: []agentfile.Env{
					{Name: "LOG_LEVEL", ValueSource: agentfile.ValueSource{Value: &envValue}},
					{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}},
				},
				MCPs: []agentfile.MCP{
					{
						Name: "time",
						Stdio: &agentfile.StdioMCP{
							Command: []string{"uv", "tool", "run", "mcp-server-time"},
							Envs: []agentfile.Env{
								{Name: "EXAMPLE", ValueSource: agentfile.ValueSource{Value: &envValue}},
								{Name: "GITHUB_PERSONAL_ACCESS_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}},
							},
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

	effective, err := os.ReadFile(filepath.Join(contextDir, "agentfile", "agentfile.effective.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(effective), "name: GITHUB_TOKEN") || !strings.Contains(string(effective), "runtimeEnv:") {
		t.Fatalf("effective agentfile missing runtimeEnv pointer:\n%s", effective)
	}
	if !strings.Contains(string(effective), "value: info") {
		t.Fatalf("effective agentfile missing literal value:\n%s", effective)
	}
	if strings.Contains(string(effective), "value: null") {
		t.Fatalf("effective agentfile has value: null for runtime entries:\n%s", effective)
	}
	configPath := filepath.Join(contextDir, "agentfile", "codex", "home", ".codex", "config.toml")
	assertFileContains(t, configPath, "project_doc_max_bytes = 0")
	assertFileContains(t, configPath, `model_instructions_file = "/agent/agentfile/system-prompt.md"`)
	assertFileContains(t, configPath, `[mcp_servers."time"]`)
	assertFileContains(t, configPath, `"GITHUB_PERSONAL_ACCESS_TOKEN" = "__AGENTFILE_REF_GITHUB_TOKEN__"`)
	entrypoint := filepath.Join(contextDir, "entrypoint")
	assertFileContains(t, entrypoint, `: "${GITHUB_TOKEN?agentfile: environment variable GITHUB_TOKEN is required}"`)
	assertFileContains(t, entrypoint, `AGENTFILE_ESC_GITHUB_TOKEN=$(printf '%s' "$GITHUB_TOKEN" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/[\\&,]/\\&/g')`)
	assertFileContains(t, entrypoint, `sed 's,__AGENTFILE_REF_GITHUB_TOKEN__,'"$AGENTFILE_ESC_GITHUB_TOKEN"',g' '/agent/agentfile/codex/home/.codex/config.toml' > '/agent/agentfile/codex/home/.codex/config.toml.tmp' && mv '/agent/agentfile/codex/home/.codex/config.toml.tmp' '/agent/agentfile/codex/home/.codex/config.toml'`)
	assertFileContains(t, entrypoint, `if [ -z "${GH_TOKEN+x}" ]; then export GH_TOKEN="${GITHUB_TOKEN}"; fi`)
	assertPathExists(t, filepath.Join(contextDir, "agentfile", "skills", "helper", "SKILL.md"))
	assertPathExists(t, filepath.Join(contextDir, "agentfile", "codex", "home", ".agents", "skills", "helper", "SKILL.md"))
	assertFileContains(t, entrypoint, `CODEX_API_KEY="$OPENAI_API_KEY"`)
	assertFileContains(t, entrypoint, `if [ -z "${LOG_LEVEL+x}" ]; then export LOG_LEVEL='info'; fi`)
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
							URL: "https://example.com/mcp",
							Headers: []agentfile.Header{
								{Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "SEARCH_MCP_AUTH"}}},
								{Name: "X-Static", ValueSource: agentfile.ValueSource{Value: &headerValue}},
							},
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

	mcpPath := filepath.Join(contextDir, "agentfile", "claudecode", "mcp.json")
	assertFileContains(t, mcpPath, `"Authorization": "__AGENTFILE_REF_SEARCH_MCP_AUTH__"`)
	assertFileContains(t, mcpPath, `"X-Static": "Bearer token"`)
	assertPathExists(t, filepath.Join(contextDir, "agentfile", "claudecode", "home"))
	entrypoint := filepath.Join(contextDir, "entrypoint")
	assertFileContains(t, entrypoint, `: "${SEARCH_MCP_AUTH?agentfile: environment variable SEARCH_MCP_AUTH is required}"`)
	assertFileContains(t, entrypoint, `sed 's,__AGENTFILE_REF_SEARCH_MCP_AUTH__,'"$AGENTFILE_ESC_SEARCH_MCP_AUTH"',g' '/agent/agentfile/claudecode/mcp.json' > '/agent/agentfile/claudecode/mcp.json.tmp' && mv '/agent/agentfile/claudecode/mcp.json.tmp' '/agent/agentfile/claudecode/mcp.json'`)
	assertFileContains(t, entrypoint, "--bare")
	assertFileContains(t, entrypoint, "--mcp-config /agent/agentfile/claudecode/mcp.json")
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
