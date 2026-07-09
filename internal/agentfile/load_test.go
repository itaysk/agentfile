package agentfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesDiscoveryAndDefaults(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
`)
	writeTestFile(t, filepath.Join(projectDir, "prompt.md"), "say hi")
	writeTestFile(t, filepath.Join(projectDir, "system-prompt.md"), "be concise")
	writeTestFile(t, filepath.Join(projectDir, "skills", "world", "SKILL.md"), `---
name: world
description: Test skill
---
body
`)

	project, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := project.DefaultImageTag(); got != "hello:latest" {
		t.Fatalf("default tag = %q, want hello:latest", got)
	}
	if project.AgentFile.Spec.Prompt == nil || project.AgentFile.Spec.Prompt.FS == nil {
		t.Fatalf("expected prompt.md to be discovered as filesystem prompt")
	}
	if got := project.AgentFile.Spec.Prompt.FS.Path; got != "prompt.md" {
		t.Fatalf("prompt path = %q, want prompt.md", got)
	}
	if project.AgentFile.Spec.SystemPrompt == nil || project.AgentFile.Spec.SystemPrompt.FS == nil {
		t.Fatalf("expected system-prompt.md to be discovered as filesystem system prompt")
	}
	if got := project.AgentFile.Spec.SystemPrompt.FS.Path; got != "system-prompt.md" {
		t.Fatalf("system prompt path = %q, want system-prompt.md", got)
	}
	if len(project.AgentFile.Spec.Skills) != 1 {
		t.Fatalf("skills length = %d, want 1", len(project.AgentFile.Spec.Skills))
	}

	resolver, err := NewResolver(project.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	assets, err := resolver.ResolveProject(project)
	if err != nil {
		t.Fatalf("ResolveProject returned error: %v", err)
	}
	if !assets.HasPrompt || assets.Prompt != "say hi" {
		t.Fatalf("prompt = (%v, %q), want discovered text", assets.HasPrompt, assets.Prompt)
	}
	if !assets.HasSystemPrompt || assets.SystemPrompt != "be concise" {
		t.Fatalf("system prompt = (%v, %q), want discovered text", assets.HasSystemPrompt, assets.SystemPrompt)
	}
	if len(assets.Skills) != 1 || assets.Skills[0].Name != "world" {
		t.Fatalf("resolved skills = %#v, want world", assets.Skills)
	}
}

func TestLoadResolvesRelativeFileFromWorkingDirectory(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
`)

	t.Chdir(projectDir)

	project, err := Load("agentfile.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got, err := filepath.EvalSymlinks(project.AgentfilePath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("agentfile path = %q, want %q", got, want)
	}
}

func TestLoadSkipsExplicitConventionalSkillsAndSortsDiscoveredSkills(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  skills:
    - fs:
        path: skills/greet/
`)
	writeTestFile(t, filepath.Join(projectDir, "skills", "zeta", "SKILL.md"), "---\nname: zeta\n---\n")
	writeTestFile(t, filepath.Join(projectDir, "skills", "greet", "SKILL.md"), "---\nname: greet\n---\n")
	writeTestFile(t, filepath.Join(projectDir, "skills", "alpha", "SKILL.md"), "---\nname: alpha\n---\n")

	project, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got := skillSourcePaths(project.AgentFile.Spec.Skills)
	want := []string{"skills/greet/", "skills/alpha", "skills/zeta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("skills = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  madeUp: true
`)

	_, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err == nil {
		t.Fatal("Load succeeded, want unknown field error")
	}
	if !strings.Contains(err.Error(), "field madeUp not found") {
		t.Fatalf("error = %q, want unknown field detail", err)
	}
}

func TestLoadDefaultsExplicitEmptyVersion(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
  version: ""
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
`)

	project, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := project.AgentFile.Metadata.Version; got != DefaultVersion {
		t.Fatalf("metadata.version = %q, want %q", got, DefaultVersion)
	}
}

func TestLoadRejectsMissingEnvValue(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  envs:
    - name: LOG_LEVEL
`)

	_, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err == nil {
		t.Fatal("Load succeeded, want missing env value error")
	}
	if !strings.Contains(err.Error(), "spec.envs[0] must set exactly one of value or runtimeEnv") {
		t.Fatalf("error = %q, want env value detail", err)
	}
}

func TestLoadParsesRuntimeEnv(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  envs:
    - name: GH_TOKEN
      runtimeEnv:
        name: GITHUB_TOKEN
  mcps:
    - name: search
      http:
        url: https://example.com/mcp
        headers:
          - name: Authorization
            runtimeEnv:
              name: SEARCH_MCP_AUTH
`)

	project, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := project.AgentFile.Spec.Envs[0].RuntimeEnv.Name; got != "GITHUB_TOKEN" {
		t.Fatalf("env runtimeEnv name = %q, want GITHUB_TOKEN", got)
	}
	if got := project.AgentFile.Spec.MCPs[0].HTTP.Headers[0].RuntimeEnv.Name; got != "SEARCH_MCP_AUTH" {
		t.Fatalf("header runtimeEnv name = %q, want SEARCH_MCP_AUTH", got)
	}
}

func TestLoadRejectsUnknownRuntimeEnvField(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  envs:
    - name: GH_TOKEN
      runtimeEnv:
        name: GITHUB_TOKEN
        madeUp: true
`)

	_, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err == nil {
		t.Fatal("Load succeeded, want unknown field error")
	}
	if !strings.Contains(err.Error(), "field madeUp not found") {
		t.Fatalf("error = %q, want unknown field detail", err)
	}
}

func TestValidateRejectsUnsupportedHarnessProvider(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    codex: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
`)

	_, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err == nil {
		t.Fatal("Load succeeded, want unsupported combination error")
	}
	if !strings.Contains(err.Error(), "codex harness supports openai") {
		t.Fatalf("error = %q, want codex provider detail", err)
	}
}

func TestValidateRejectsDuplicateMCPNames(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  mcps:
    - name: search
      stdio:
        command: ["tool"]
    - name: search
      http:
        url: https://example.com/mcp
`)

	_, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err == nil {
		t.Fatal("Load succeeded, want duplicate MCP name error")
	}
	if !strings.Contains(err.Error(), `spec.mcps[1].name "search" must be unique within spec.mcps`) {
		t.Fatalf("error = %q, want duplicate MCP name detail", err)
	}
}

func TestResolveRejectsDuplicateSkillNames(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    claudecode: {}
  llm:
    anthropic:
      model: claude-haiku-4-5
  skills:
    - fs:
        path: skills/one
    - fs:
        path: skills/two
`)
	writeTestFile(t, filepath.Join(projectDir, "skills", "one", "SKILL.md"), "---\nname: same\n---\n")
	writeTestFile(t, filepath.Join(projectDir, "skills", "two", "SKILL.md"), "---\nname: same\n---\n")

	project, err := Load(filepath.Join(projectDir, "agentfile.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	resolver, err := NewResolver(project.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	_, err = resolver.ResolveProject(project)
	if err == nil {
		t.Fatal("ResolveProject succeeded, want duplicate skill error")
	}
	if !strings.Contains(err.Error(), `skill name "same", which must be unique within spec.skills`) {
		t.Fatalf("error = %q, want duplicate skill detail", err)
	}
}

func skillSourcePaths(sources []Source) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		if source.FS != nil {
			paths = append(paths, source.FS.Path)
		}
	}
	return paths
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
