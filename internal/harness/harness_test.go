package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
)

func TestPrepareCodexRendersRelocatableConfigAndCommand(t *testing.T) {
	runtimeName := "MCP_TOKEN"
	literal := "info"
	prompt := agentfile.TextSource("default prompt")
	system := agentfile.TextSource("standing instructions")
	af := baseAgentfile()
	af.Spec.Prompt = &prompt
	af.Spec.SystemPrompt = &system
	af.Spec.Envs = []agentfile.Env{
		{Name: "LOG_LEVEL", ValueSource: agentfile.ValueSource{Value: &literal}},
		{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}},
	}
	af.Spec.MCPs = []agentfile.MCP{{
		Name: "search",
		HTTP: &agentfile.HTTPMCP{URL: "https://example.com/mcp", Headers: []agentfile.Header{{
			Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: runtimeName}},
		}}},
	}}
	unpacked := writeUnpackedBundle(t, af, &agentfile.ResolvedAssets{
		Prompt: "default prompt", HasPrompt: true, SystemPrompt: "standing instructions", HasSystemPrompt: true,
	})
	workspace := filepath.Join(t.TempDir(), "work space")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	override := "invocation prompt"
	profile := filepath.Join(t.TempDir(), "profile")
	command, err := Prepare(unpacked, profile, Invocation{
		Mode: ModeOneShot, Workspace: workspace, Prompt: &override, Model: "gpt-5.2",
		Env: map[string]string{"GITHUB_TOKEN": "github-secret", runtimeName: `Bearer "token"`, "LOG_LEVEL": "debug"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Executable != "codex" || command.Dir != workspace || !slices.Contains(command.Args, override) || !slices.Contains(command.Args, "gpt-5.2") {
		t.Fatalf("command = %#v", command)
	}
	if command.Env["LOG_LEVEL"] != "debug" || command.Env["GH_TOKEN"] != "github-secret" {
		t.Fatalf("environment defaults/mapping = %#v", command.Env)
	}
	configPath := filepath.Join(command.Env["CODEX_HOME"], "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{workspace, filepath.Join(unpacked.Root, "system-prompt.md"), `Bearer \"token\"`} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("config = %s, want %q", config, want)
		}
	}
	template, _ := os.ReadFile(filepath.Join(unpacked.Root, "harness", "codex", "config.toml.tmpl"))
	if !strings.Contains(string(template), bundle.WorkspaceToken) || strings.Contains(string(template), "github-secret") {
		t.Fatalf("bundle template was modified or contains a secret: %s", template)
	}
}

func TestPrepareHarnessCommandsAndModes(t *testing.T) {
	claude := agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}
	codex := agentfile.Harness{Codex: &agentfile.EmptyObject{}}
	pi := agentfile.Harness{Pi: &agentfile.EmptyObject{}}
	anthropic := agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-model"}}
	openai := agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-model"}}
	openrouter := agentfile.LLM{OpenRouter: &agentfile.ModelProvider{Model: "provider/model"}}
	for _, tt := range []struct {
		name       string
		harness    agentfile.Harness
		llm        agentfile.LLM
		mode       Mode
		executable string
		wantArgs   []string
	}{
		{"claude oneshot", claude, anthropic, ModeOneShot, "claude", []string{"--print", "--model", "claude-model", "--dangerously-skip-permissions", "hello"}},
		{"claude tui", claude, anthropic, ModeTUI, "claude", []string{"--model", "claude-model", "--dangerously-skip-permissions"}},
		{"claude acp", claude, anthropic, ModeACP, "claude", []string{"--output-format", "stream-json", "--verbose", "--model", "claude-model", "--dangerously-skip-permissions", "--input-format", "stream-json", "--include-partial-messages"}},
		{"codex oneshot", codex, openai, ModeOneShot, "codex", []string{"exec", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-model", "hello"}},
		{"codex tui", codex, openai, ModeTUI, "codex", []string{"--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-model"}},
		{"codex acp", codex, openai, ModeACP, "codex", []string{"--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-model", "app-server"}},
		{"pi oneshot", pi, openrouter, ModeOneShot, "pi", []string{"-p", "--provider", "openrouter", "--model", "provider/model", "--no-context-files", "--system-prompt", "standing", "hello"}},
		{"pi tui", pi, openrouter, ModeTUI, "pi", []string{"--provider", "openrouter", "--model", "provider/model", "--no-context-files", "--system-prompt", "standing"}},
		{"pi acp", pi, openrouter, ModeACP, "pi", []string{"--mode", "rpc", "--provider", "openrouter", "--model", "provider/model", "--no-context-files", "--system-prompt", "standing"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			af := baseAgentfile()
			af.Spec.Harness, af.Spec.LLM = tt.harness, tt.llm
			assets := &agentfile.ResolvedAssets{Prompt: "hello", HasPrompt: true}
			if tt.harness.Pi != nil {
				system := agentfile.TextSource("standing")
				af.Spec.SystemPrompt = &system
				assets.SystemPrompt, assets.HasSystemPrompt = "standing", true
			}
			unpacked := writeUnpackedBundle(t, af, assets)
			workspace := t.TempDir()
			command, err := Prepare(unpacked, filepath.Join(t.TempDir(), "profile"), Invocation{Mode: tt.mode, Workspace: workspace, Env: map[string]string{}})
			if err != nil {
				t.Fatal(err)
			}
			if command.Executable != tt.executable || !reflect.DeepEqual(command.Args, tt.wantArgs) || command.Dir != workspace {
				t.Fatalf("command = %#v, want executable=%s args=%#v dir=%s", command, tt.executable, tt.wantArgs, workspace)
			}
			for _, name := range []string{"AGENTFILE_MODEL", "AGENTFILE_PROVIDER", "AGENTFILE_PROMPT"} {
				if _, exists := command.Env[name]; exists {
					t.Fatalf("command environment contains internal variable %s: %#v", name, command.Env)
				}
			}
			if tt.executable == "claude" {
				if _, exists := command.Env["IS_SANDBOX"]; exists {
					t.Fatalf("host command environment sets IS_SANDBOX: %#v", command.Env)
				}
			}
		})
	}
}

func TestPrepareRuntimeEnvironment(t *testing.T) {
	af := baseAgentfile()
	af.Spec.Envs = []agentfile.Env{{Name: "TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "REQUIRED_TOKEN"}}}}
	unpacked := writeUnpackedBundle(t, af, &agentfile.ResolvedAssets{Prompt: "hello", HasPrompt: true})
	profile := filepath.Join(t.TempDir(), "profile")
	_, err := Prepare(unpacked, profile, Invocation{Mode: ModeOneShot, Workspace: t.TempDir(), Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "environment variable REQUIRED_TOKEN is required") {
		t.Fatalf("Prepare error = %v", err)
	}
	command, err := Prepare(unpacked, profile, Invocation{Mode: ModeOneShot, Workspace: t.TempDir(), Env: map[string]string{"REQUIRED_TOKEN": ""}})
	if err != nil || command.Env["TOKEN"] != "" {
		t.Fatalf("empty runtime value: command=%#v err=%v", command, err)
	}
}

func TestPrepareRejectsInvalidInvocation(t *testing.T) {
	unpacked := writeUnpackedBundle(t, baseAgentfile(), &agentfile.ResolvedAssets{Prompt: "hello", HasPrompt: true})
	if _, err := Prepare(nil, "profile", Invocation{}); err == nil || !strings.Contains(err.Error(), "unpacked bundle is required") {
		t.Fatalf("Prepare unpacked bundle error = %v", err)
	}
	if _, err := Prepare(unpacked, "", Invocation{}); err == nil || !strings.Contains(err.Error(), "profile root is required") {
		t.Fatalf("Prepare profile error = %v", err)
	}
	for _, tt := range []struct {
		name       string
		invocation Invocation
		want       string
	}{
		{"mode", Invocation{Mode: "bad", Workspace: t.TempDir(), Env: map[string]string{}}, "unsupported execution mode"},
		{"workspace", Invocation{Mode: ModeOneShot, Env: map[string]string{}}, "workspace is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Prepare(unpacked, filepath.Join(t.TempDir(), "profile"), tt.invocation); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Prepare error = %v, want %q", err, tt.want)
			}
		})
	}
	promptless := *unpacked
	promptless.Manifest.Assets.Prompt = ""
	if _, err := Prepare(&promptless, filepath.Join(t.TempDir(), "profile"), Invocation{Mode: ModeOneShot, Workspace: t.TempDir(), Env: map[string]string{}}); err == nil || !strings.Contains(err.Error(), "effective prompt") {
		t.Fatalf("Prepare prompt error = %v", err)
	}
	if _, err := NewCommand(unpacked.Manifest, CommandOptions{Mode: "bad"}); err == nil || !strings.Contains(err.Error(), "unsupported execution mode") {
		t.Fatalf("NewCommand mode error = %v", err)
	}
}

func TestPrepareClaudeRendersConfigAndSkills(t *testing.T) {
	secret := `Bearer "token"`
	skillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	af := baseAgentfile()
	af.Spec.Harness = agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}
	af.Spec.LLM = agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-model"}}
	af.Spec.MCPs = []agentfile.MCP{
		{Name: "local", Stdio: &agentfile.StdioMCP{Command: []string{"server", "--stdio"}, Envs: []agentfile.Env{{Name: "TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "MCP_TOKEN"}}}}}},
		{Name: "remote", HTTP: &agentfile.HTTPMCP{URL: "https://example.com/mcp", Headers: []agentfile.Header{{Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "MCP_TOKEN"}}}}}},
	}
	unpacked := writeUnpackedBundle(t, af, &agentfile.ResolvedAssets{
		Prompt: "hello", HasPrompt: true,
		Skills: []agentfile.ResolvedSkill{{Name: "demo", Dir: skillDir}},
	})
	profile := filepath.Join(t.TempDir(), "profile")
	command, err := Prepare(unpacked, profile, Invocation{Mode: ModeOneShot, Workspace: t.TempDir(), Env: map[string]string{"MCP_TOKEN": secret}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(profile, "claudecode", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		MCPServers map[string]struct {
			Env     map[string]string `json:"env"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.MCPServers["local"].Env["TOKEN"] != secret || config.MCPServers["remote"].Headers["Authorization"] != secret {
		t.Fatalf("rendered config = %#v", config)
	}
	script := filepath.Join(command.Env["HOME"], ".claude", "skills", "demo", "run.sh")
	info, err := os.Stat(script)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("staged skill mode: info=%v err=%v", info, err)
	}
	template, err := os.ReadFile(filepath.Join(unpacked.Root, "harness", "claudecode", "mcp.json.tmpl"))
	if err != nil || strings.Contains(string(template), secret) || !strings.Contains(string(template), agentfile.RefTokenPrefix+"MCP_TOKEN__") {
		t.Fatalf("template = %q err=%v", template, err)
	}
}

func baseAgentfile() agentfile.AgentFile {
	return agentfile.AgentFile{
		APIVersion: agentfile.APIVersion,
		Kind:       agentfile.Kind,
		Metadata:   agentfile.Metadata{Name: "test", Version: "latest"},
		Spec: agentfile.Spec{
			Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
			LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5"}},
		},
	}
}

func writeUnpackedBundle(t *testing.T, af agentfile.AgentFile, assets *agentfile.ResolvedAssets) *bundle.Unpacked {
	t.Helper()
	bundleRoot := t.TempDir()
	manifest, err := bundle.WriteLayout(bundleRoot, &agentfile.Project{ProjectDir: bundleRoot, AgentFile: af}, assets)
	if err != nil {
		t.Fatal(err)
	}
	return &bundle.Unpacked{Root: bundleRoot, Manifest: *manifest}
}
