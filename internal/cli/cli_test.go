package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/config"
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
	if !strings.Contains(stdout.String(), "--tui") {
		t.Fatalf("stdout = %q, want --tui in run usage", stdout.String())
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

func TestRunErrorPrefixesOnce(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"run", []string{"run", "--bad"}, "unknown run argument"},
		{"agents", []string{"agents", "bad"}, "unknown agents command"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(tt.args, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if got := strings.Count(stderr.String(), "af:"); got != 1 {
				t.Fatalf("stderr = %q, want one af prefix", stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestParseBuildFlagsSupportsShortFileEquals(t *testing.T) {
	options := buildFlags{}
	if err := parseBuildFlags([]string{"-f=agentfile.yaml"}, &options); err != nil {
		t.Fatalf("parseBuildFlags returned error: %v", err)
	}
	if options.file != "agentfile.yaml" {
		t.Fatalf("file = %q, want agentfile.yaml", options.file)
	}
}

func TestParseRunFlagsSupportsPromptOverride(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"cc", "--prompt", "say hi"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.name != "cc" {
		t.Fatalf("name = %q, want cc", options.name)
	}
	if options.prompt == nil || *options.prompt != "say hi" {
		t.Fatalf("prompt = %v, want say hi", options.prompt)
	}
}

func TestParseRunFlagsSupportsTUIWithoutPrompt(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"cc", "--tui"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if !options.tui {
		t.Fatal("tui = false, want true")
	}
	if err := parseRunFlags([]string{"--tui", "--prompt", "say hi"}, &runFlags{env: map[string]string{}}); err == nil || !strings.Contains(err.Error(), "--prompt cannot be used with --tui") {
		t.Fatalf("parseRunFlags TUI/prompt error = %v, want conflict", err)
	}
}

func TestParseRunFlagsSupportsModelOverride(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"--model=claude-sonnet-4-5"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.model != "claude-sonnet-4-5" {
		t.Fatalf("model = %q, want claude-sonnet-4-5", options.model)
	}

	for _, args := range [][]string{{"--model="}, {"--llm.anthropic.model", "claude-sonnet-4-5"}, {"--prompt.text", "hi"}, {"--harness.image", "example/image"}, {"--systemPrompt", "x"}} {
		if err := parseRunFlags(args, &runFlags{env: map[string]string{}}); err == nil {
			t.Fatalf("parseRunFlags(%q) accepted removed generic override", args)
		}
	}
}

func TestParseRunFlagsSupportsImage(t *testing.T) {
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags([]string{"--image=acme/triage:1.2"}, &options); err != nil {
		t.Fatalf("parseRunFlags returned error: %v", err)
	}
	if options.image != "acme/triage:1.2" {
		t.Fatalf("image = %q, want acme/triage:1.2", options.image)
	}

	for _, args := range [][]string{
		{"--image="},
		{"--image"},
		{"--image", "acme/triage:1.2", "triage"},
		{"--image", "acme/triage:1.2", "--file", "agentfile.yaml"},
		{"triage", "--file", "agentfile.yaml"},
	} {
		if err := parseRunFlags(args, &runFlags{env: map[string]string{}}); err == nil {
			t.Fatalf("parseRunFlags(%q) accepted invalid image selection", args)
		}
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
	registryPath, err := config.RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	registryData, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(registryData), "defaultImageTag") {
		t.Fatalf("registry = %q, want no defaultImageTag", registryData)
	}
	writeCLITestFile(t, filepath.Join(projectDir, "agentfile.yaml"), `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: hello
  version: "2"
spec:
  harness:
    codex: {}
  llm:
    openai:
      model: gpt-5-mini
`)
	project, tag, _, _, err := loadRunSelection(runFlags{name: "alias"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "" {
		t.Fatalf("registered run tag = %q, want build default", tag)
	}
	if got := project.DefaultImageTag(); got != "hello:2" {
		t.Fatalf("registered run image tag = %q, want hello:2", got)
	}

	var listOut bytes.Buffer
	var listErr bytes.Buffer
	code = Run([]string{"agents", "list"}, &listOut, &listErr)
	if code != 0 {
		t.Fatalf("list exit code = %d, stderr = %q", code, listErr.String())
	}
	if !strings.Contains(listOut.String(), "alias") || !strings.Contains(listOut.String(), "hello:2") {
		t.Fatalf("list stdout = %q, want registered alias and tag", listOut.String())
	}
}

func TestRegisterImageListAndRunValidation(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("AppData", configHome)
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var registerOut bytes.Buffer
	var registerErr bytes.Buffer
	code := Run([]string{"agents", "register", "--image", "acme/triage:1.2"}, &registerOut, &registerErr)
	if code != 0 {
		t.Fatalf("register exit code = %d, stderr = %q", code, registerErr.String())
	}
	if !strings.Contains(registerOut.String(), "Registered image-agent") {
		t.Fatalf("register stdout = %q, want image label name", registerOut.String())
	}
	registryPath, err := config.RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	registryData, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(registryData), `"image": "acme/triage:1.2"`) || strings.Contains(string(registryData), "agentfilePath") {
		t.Fatalf("registry = %q, want image entry only", registryData)
	}

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var listOut bytes.Buffer
	var listErr bytes.Buffer
	code = Run([]string{"agents", "list"}, &listOut, &listErr)
	if code != 0 {
		t.Fatalf("list exit code = %d, stderr = %q", code, listErr.String())
	}
	if !strings.Contains(listOut.String(), "image-agent") || !strings.Contains(listOut.String(), "acme/triage:1.2") || !strings.Contains(listOut.String(), "-") {
		t.Fatalf("list stdout = %q, want image entry with no agentfile", listOut.String())
	}
	if log := readCLILog(t, logPath); log != "" {
		t.Fatalf("agents list called docker:\n%s", log)
	}

	project, image, runtimeEnvNames, harness, err := loadRunSelection(runFlags{name: "image-agent"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if project != nil || image != "acme/triage:1.2" || strings.Join(runtimeEnvNames, ",") != "GITHUB_TOKEN" || harness != "claudecode" {
		t.Fatalf("loadRunSelection = (%#v, %q, %#v, %q), want image selection", project, image, runtimeEnvNames, harness)
	}

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var overrideOut bytes.Buffer
	var overrideErr bytes.Buffer
	code = Run([]string{"run", "image-agent", "--prompt", "say hi", "--model", "gpt-5"}, &overrideOut, &overrideErr)
	if code != 0 {
		t.Fatalf("run override exit = %d, stderr = %q", code, overrideErr.String())
	}
	if args := readCLILog(t, logPath); !strings.Contains(args, "-e AGENTFILE_MODEL=gpt-5") || !strings.Contains(args, "-e AGENTFILE_PROMPT=say hi") {
		t.Fatalf("docker log = %q, want prompt and model overrides", args)
	}

	t.Setenv("DOCKER_INSPECT_FAIL_ONCE", filepath.Join(t.TempDir(), "fail-once"))
	var pullOut bytes.Buffer
	var pullErr bytes.Buffer
	code = Run([]string{"run", "image-agent"}, &pullOut, &pullErr)
	if code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, pullErr.String())
	}
	if !strings.Contains(pullErr.String(), "pulling acme/triage:1.2") {
		t.Fatalf("run stderr = %q, want pull progress without --debug", pullErr.String())
	}

	t.Setenv("DOCKER_INSPECT_FAIL_ONCE", filepath.Join(t.TempDir(), "fail-once"))
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var missOut bytes.Buffer
	var missErr bytes.Buffer
	code = Run([]string{"agents", "register", "--image", "acme/other:1"}, &missOut, &missErr)
	if code != 1 || !strings.Contains(missErr.String(), "docker pull the image first") {
		t.Fatalf("register exit = %d, stderr = %q, want pull hint", code, missErr.String())
	}
	if strings.Contains(readCLILog(t, logPath), "pull ") {
		t.Fatalf("register pulled the image:\n%s", readCLILog(t, logPath))
	}
}

func TestRunImageAdHoc(t *testing.T) {
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_TOKEN", "host-token")
	workspace := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{
		"run", "--image", "acme/triage:1.2",
		"--prompt", "say hi", "--model", "gpt-5",
		"--workspace", workspace, "--env", "EXTRA=value",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, stderr.String())
	}
	log := readCLILog(t, logPath)
	for _, want := range []string{
		"image inspect --format",
		"-e AGENTFILE_MODEL=gpt-5",
		"-e AGENTFILE_PROMPT=say hi",
		"-e EXTRA=value",
		"-e GITHUB_TOKEN=host-token",
		"-v " + workspace + ":/agent/workspace",
		"acme/triage:1.2",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker log = %q, want %q", log, want)
		}
	}
	if strings.Contains(log, " build ") {
		t.Fatalf("ad hoc image run built an image:\n%s", log)
	}
}

func TestRegisterRejectsFileAndImage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"agents", "register", "--file", "agentfile.yaml", "--image", "acme/triage:1.2"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--file and --image cannot be used together") {
		t.Fatalf("stderr = %q, want --file/--image conflict", stderr.String())
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

func installCLIFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	dockerPath := filepath.Join(binDir, "docker")
	writeCLITestFile(t, dockerPath, `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  if [ -n "${DOCKER_INSPECT_FAIL_ONCE:-}" ] && [ ! -f "$DOCKER_INSPECT_FAIL_ONCE" ]; then
    touch "$DOCKER_INSPECT_FAIL_ONCE"
    exit 1
  fi
  cat <<'JSON'
{"build.agentfile.metadata":"{\"name\":\"image-agent\",\"version\":\"latest\"}","build.agentfile.runtimeEnv":"[\"GITHUB_TOKEN\"]","build.agentfile.harness":"claudecode"}
JSON
  exit 0
fi
if [ "$1" = "pull" ]; then
  echo "pulling $2" >&2
  exit 0
fi
exit 0
`)
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_ARGS_LOG", logPath)
	return dockerPath, logPath
}

func readCLILog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
