package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpExitsZero(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"run", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "usage: af run") {
		t.Fatalf("stdout = %q, want run usage", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestBuildRejectsPositionalArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"build", "agentfile.yaml"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "build does not accept positional arguments") {
		t.Fatalf("stderr = %q, want positional argument error", stderr.String())
	}
}

func TestParseRunFlagsSupportsPromptOverrideAlias(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"cc", "--prompt", "say hi"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.name != "cc" {
		t.Fatalf("name = %q, want cc", options.name)
	}
	if len(options.mutations) != 1 {
		t.Fatalf("mutations = %#v, want one prompt mutation", options.mutations)
	}
	if options.mutations[0].path != "prompt" || options.mutations[0].value != "say hi" {
		t.Fatalf("mutation = %#v, want prompt text", options.mutations[0])
	}
}

func TestParseRunFlagsWorkspaceShorthands(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"--workspace", "."}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.workspace != cwd {
		t.Fatalf("workspace = %q, want cwd %q", options.workspace, cwd)
	}

	options = runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"--ws=/tmp/work"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.workspace != "/tmp/work" {
		t.Fatalf("workspace = %q, want /tmp/work", options.workspace)
	}

	if err := parseRunFlags([]string{"--workspace="}, &runFlags{env: map[string]string{}}); err == nil {
		t.Fatal("parseRunFlags accepted empty --workspace, want value error")
	}
}

func TestParseRunFlagsDebug(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"--debug"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if !options.debug {
		t.Fatal("debug = false, want true")
	}
}

func TestRegisterAndListUseConfigRegistry(t *testing.T) {
	// Isolate os.UserConfigDir() across platforms: Linux honors XDG_CONFIG_HOME,
	// macOS/Windows derive from HOME/AppData.
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("AppData", configHome)
	projectDir := t.TempDir()
	writeCLITestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
spec:
  harness:
    codex: {}
  llm:
    openai:
      model: gpt-5-mini
`)

	var registerOut bytes.Buffer
	var registerErr bytes.Buffer
	code := Run([]string{"agents", "register", "alias", "-f", filepath.Join(projectDir, "agentfile.yaml")}, &registerOut, &registerErr)
	if code != 0 {
		t.Fatalf("register exit code = %d, stderr = %q", code, registerErr.String())
	}
	if !strings.Contains(registerOut.String(), "Registered alias") {
		t.Fatalf("register stdout = %q, want alias", registerOut.String())
	}

	var listOut bytes.Buffer
	var listErr bytes.Buffer
	code = Run([]string{"agents", "list"}, &listOut, &listErr)
	if code != 0 {
		t.Fatalf("list exit code = %d, stderr = %q", code, listErr.String())
	}
	if !strings.Contains(listOut.String(), "alias") || !strings.Contains(listOut.String(), "hello:latest") {
		t.Fatalf("list stdout = %q, want registered alias and tag", listOut.String())
	}
}

func writeCLITestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
