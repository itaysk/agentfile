package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func TestRunSkipsDockerStdinForDevNull(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	code, err := Run(context.Background(), Options{
		Project:      runnerTestProject(t),
		DockerBinary: dockerPath,
		Stdin:        devNull,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	if strings.Contains(" "+dockerRunArgs(t, logPath)+" ", " -i ") {
		t.Fatalf("docker run args include -i for /dev/null stdin")
	}
}

func TestRunRejectsUnknownMode(t *testing.T) {
	code, err := Run(context.Background(), Options{Project: runnerTestProject(t), Mode: "bad"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "unsupported run mode") {
		t.Fatalf("Run = (%d, %v), want unsupported mode error", code, err)
	}
}

func TestRunForwardsRedirectedStdin(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	stdinLog := filepath.Join(t.TempDir(), "stdin.log")
	t.Setenv("DOCKER_STDIN_LOG", stdinLog)
	inputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(inputPath, []byte("piped context"), 0o644); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	code, err := Run(context.Background(), Options{
		Project:      runnerTestProject(t),
		DockerBinary: dockerPath,
		Stdin:        input,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	if !strings.Contains(" "+dockerRunArgs(t, logPath)+" ", " -i ") {
		t.Fatalf("docker run args do not include -i for explicit stdin")
	}
	data, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "piped context" {
		t.Fatalf("stdin = %q, want piped context", string(data))
	}
}

func TestRunTUILaunchesPromptlessClaudeWithTTY(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	project := runnerTestProject(t)
	project.AgentFile.Spec.Harness = agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}
	project.AgentFile.Spec.LLM = agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-haiku-4-5"}}
	project.AgentFile.Spec.Prompt = nil

	code, err := Run(context.Background(), Options{
		Project:      project,
		Mode:         RunModeTUI,
		Env:          map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "test-token"},
		DockerBinary: dockerPath,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run TUI = (%d, %v), want success", code, err)
	}
	args := dockerRunArgs(t, logPath)
	for _, want := range []string{"-it", "-e AGENTFILE_RUN_MODE=tui", "-e CLAUDE_CODE_OAUTH_TOKEN=test-token"} {
		if !strings.Contains(args, want) {
			t.Fatalf("docker run args = %q, want %q", args, want)
		}
	}
	if strings.Contains(args, "AGENTFILE_PROMPT") {
		t.Fatalf("docker run args = %q, want no prompt in TUI mode", args)
	}
	if !strings.Contains(dockerLog(t, logPath), "--label build.agentfile.harness=claudecode") {
		t.Fatalf("docker build log = %q, want harness label", dockerLog(t, logPath))
	}
}

func TestRunTUIRejectsPromptAndLegacyImages(t *testing.T) {
	prompt := "not supported"
	for _, tt := range []struct {
		name    string
		options Options
		want    string
	}{
		{
			name: "prompt",
			options: Options{
				Project: runnerTestProject(t), Mode: RunModeTUI, Prompt: &prompt,
			},
			want: "--prompt cannot be used with --tui",
		},
		{
			name:    "legacy image",
			options: Options{Image: "acme/legacy:1", Mode: RunModeTUI},
			want:    "predates TUI support",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, err := Run(context.Background(), tt.options)
			if code != 1 || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run = (%d, %v), want error containing %q", code, err, tt.want)
			}
		})
	}
}

func TestRunAddsExtraDockerArgs(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	code, err := Run(context.Background(), Options{
		Project:         runnerTestProject(t),
		DockerBinary:    dockerPath,
		Stdin:           devNull,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		extraDockerArgs: []string{"--add-host", "host.docker.internal:host-gateway"},
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	runArgs := dockerRunArgs(t, logPath)
	if !strings.Contains(runArgs, "--add-host host.docker.internal:host-gateway") {
		t.Fatalf("docker run args = %q, want extra docker args", runArgs)
	}
}

func TestRunMountsWorkspace(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	workspace := filepath.Join(t.TempDir(), "with:colon")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	code, err := Run(context.Background(), Options{
		Project:      runnerTestProject(t),
		DockerBinary: dockerPath,
		Workspace:    workspace,
		Stdin:        devNull,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	if !strings.Contains(dockerRunArgs(t, logPath), "--mount type=bind,source="+workspace+",target=/agent/workspace") {
		t.Fatalf("docker run args = %q, want workspace mount", dockerRunArgs(t, logPath))
	}
}

func TestRunWithImageSkipsBuild(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	t.Setenv("GITHUB_TOKEN", "from-host")
	image := "registry.example/agent:1.2"
	prompt := "say runtime hi"
	code, err := Run(context.Background(), Options{
		DockerBinary:    dockerPath,
		Image:           image,
		RuntimeEnvNames: []string{"GITHUB_TOKEN"},
		Prompt:          &prompt,
		Model:           "gpt-5",
		Stdin:           devNull,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	log := dockerLog(t, logPath)
	if strings.Contains(log, "build ") {
		t.Fatalf("docker log contains build despite image option:\n%s", log)
	}
	if !strings.Contains(dockerRunArgs(t, logPath), image) {
		t.Fatalf("docker run args = %q, want image ref", dockerRunArgs(t, logPath))
	}
	if !strings.Contains(dockerRunArgs(t, logPath), "-e GITHUB_TOKEN=from-host") {
		t.Fatalf("docker run args = %q, want runtime env forwarded", dockerRunArgs(t, logPath))
	}
	if !strings.Contains(dockerRunArgs(t, logPath), "-e AGENTFILE_MODEL=gpt-5") {
		t.Fatalf("docker run args = %q, want model override", dockerRunArgs(t, logPath))
	}
	if !strings.Contains(dockerRunArgs(t, logPath), "-e AGENTFILE_PROMPT=say runtime hi") {
		t.Fatalf("docker run args = %q, want prompt override", dockerRunArgs(t, logPath))
	}
}

func TestRunAcceptsRuntimePromptWithoutBuildPrompt(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	project := runnerTestProject(t)
	project.AgentFile.Spec.Prompt = nil
	prompt := "runtime prompt"

	code, err := Run(context.Background(), Options{
		Project:      project,
		Prompt:       &prompt,
		DockerBinary: dockerPath,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v), want success", code, err)
	}
	if !strings.Contains(dockerRunArgs(t, logPath), "-e AGENTFILE_PROMPT=runtime prompt") {
		t.Fatalf("docker run args = %q, want runtime prompt", dockerRunArgs(t, logPath))
	}
}

func TestRunRoutesOutput(t *testing.T) {
	for _, tt := range []struct {
		name          string
		captureStderr bool
		wantStderr    []string
	}{
		{name: "discarded stderr"},
		{name: "captured stderr", captureStderr: true, wantStderr: []string{"build stdout", "build stderr", "agent stderr"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dockerPath, _ := installFakeDocker(t)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			var runStderr io.Writer = io.Discard
			if tt.captureStderr {
				runStderr = &stderr
			}

			code, err := Run(context.Background(), Options{
				Project:      runnerTestProject(t),
				DockerBinary: dockerPath,
				Stdout:       &stdout,
				Stderr:       runStderr,
			})
			if err != nil || code != 0 {
				t.Fatalf("Run = (%d, %v), want success", code, err)
			}
			if stdout.String() != "agent stdout\n" {
				t.Fatalf("stdout = %q, want agent stdout only", stdout.String())
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr = %q, want %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestReadImageInfoReadsLabelsWithoutPulling(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)

	info, err := ReadImageInfo(context.Background(), dockerPath, "acme/triage:1.2")
	if err != nil {
		t.Fatalf("ReadImageInfo returned error: %v", err)
	}
	if info.Metadata.Name != "image-agent" {
		t.Fatalf("metadata.name = %q, want image-agent", info.Metadata.Name)
	}
	if strings.Join(info.RuntimeEnvNames, ",") != "GITHUB_TOKEN" {
		t.Fatalf("runtime env names = %#v, want GITHUB_TOKEN", info.RuntimeEnvNames)
	}
	if info.Harness != "claudecode" {
		t.Fatalf("harness = %q, want claudecode", info.Harness)
	}

	t.Setenv("DOCKER_INSPECT_FAIL_ONCE", filepath.Join(t.TempDir(), "fail-once"))
	if _, err := ReadImageInfo(context.Background(), dockerPath, "acme/triage:1.2"); err == nil {
		t.Fatal("ReadImageInfo succeeded despite failed inspect, want error")
	}
	if strings.Contains(dockerLog(t, logPath), "pull ") {
		t.Fatalf("ReadImageInfo pulled:\n%s", dockerLog(t, logPath))
	}
}

func TestPullImageStreamsProgress(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	var stderr bytes.Buffer

	if err := PullImage(context.Background(), dockerPath, "acme/triage:1.2", &stderr); err != nil {
		t.Fatalf("PullImage returned error: %v", err)
	}
	if !strings.Contains(dockerLog(t, logPath), "pull acme/triage:1.2") {
		t.Fatalf("docker log = %q, want pull", dockerLog(t, logPath))
	}
	if !strings.Contains(stderr.String(), "pulling acme/triage:1.2") {
		t.Fatalf("stderr = %q, want pull progress", stderr.String())
	}
}

func TestReadImageInfoRejectsMissingLabel(t *testing.T) {
	dockerPath, _ := installFakeDocker(t)
	t.Setenv("DOCKER_MISSING_LABEL", "1")

	_, err := ReadImageInfo(context.Background(), dockerPath, "busybox:latest")
	if err == nil || !strings.Contains(err.Error(), "missing build.agentfile.metadata label") {
		t.Fatalf("ReadImageInfo error = %v, want missing label", err)
	}
}

func TestReadImageInfoAllowsLegacyImageWithoutHarnessLabel(t *testing.T) {
	dockerPath, _ := installFakeDocker(t)
	t.Setenv("DOCKER_MISSING_HARNESS_LABEL", "1")

	info, err := ReadImageInfo(context.Background(), dockerPath, "acme/legacy:1")
	if err != nil {
		t.Fatalf("ReadImageInfo returned error for legacy image: %v", err)
	}
	if info.Harness != "" {
		t.Fatalf("harness = %q, want empty legacy value", info.Harness)
	}
}

func TestRunRejectsInvalidWorkspaceHostPathBeforeDocker(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing"), filePath} {
		code, err := Run(context.Background(), Options{
			Project:      runnerTestProject(t),
			DockerBinary: dockerPath,
			Workspace:    path,
			Stdout:       io.Discard,
			Stderr:       io.Discard,
		})
		if err == nil || code != 1 {
			t.Fatalf("Run = (%d, %v), want invalid workspace error", code, err)
		}
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("docker was called; log stat error = %v", err)
	}
}

func TestRunEnvForwardsRuntimeEnvNames(t *testing.T) {
	project := runnerTestProject(t)
	project.AgentFile.Spec.Envs = []agentfile.Env{
		{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}},
		{Name: "OTHER", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "MISSING_ON_HOST"}}},
	}
	t.Setenv("GITHUB_TOKEN", "from-host")
	os.Unsetenv("MISSING_ON_HOST")

	envs := runEnv(project.AgentFile.Spec.RuntimeEnvNames(), map[string]string{})
	if got := envs["GITHUB_TOKEN"]; got != "from-host" {
		t.Fatalf("GITHUB_TOKEN = %q, want from-host", got)
	}
	if _, ok := envs["MISSING_ON_HOST"]; ok {
		t.Fatalf("MISSING_ON_HOST forwarded despite being unset on host")
	}

	envs = runEnv(project.AgentFile.Spec.RuntimeEnvNames(), map[string]string{"GITHUB_TOKEN": "explicit"})
	if got := envs["GITHUB_TOKEN"]; got != "explicit" {
		t.Fatalf("GITHUB_TOKEN = %q, want explicit --env to win", got)
	}
}

func installFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	dockerPath := filepath.Join(binDir, "docker")
	writeRunnerTestFile(t, dockerPath, `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  if [ -n "${DOCKER_INSPECT_FAIL_ONCE:-}" ] && [ ! -f "$DOCKER_INSPECT_FAIL_ONCE" ]; then
    touch "$DOCKER_INSPECT_FAIL_ONCE"
    exit 1
  fi
  if [ "${DOCKER_MISSING_LABEL:-}" = "1" ]; then
    echo '{}'
    exit 0
  fi
  if [ "${DOCKER_MISSING_HARNESS_LABEL:-}" = "1" ]; then
    echo '{"build.agentfile.metadata":"{\"name\":\"image-agent\",\"version\":\"latest\"}","build.agentfile.runtimeEnv":"[\"GITHUB_TOKEN\"]"}'
    exit 0
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
if [ "$1" = "build" ]; then
  echo "build stdout"
  echo "build stderr" >&2
fi
if [ "$1" = "run" ] && [ -n "${DOCKER_STDIN_LOG:-}" ]; then
  cat > "$DOCKER_STDIN_LOG"
fi
if [ "$1" = "run" ]; then
  echo "agent stdout"
  echo "agent stderr" >&2
fi
exit 0
`)
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_ARGS_LOG", logPath)
	return dockerPath, logPath
}

func dockerLog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func dockerRunArgs(t *testing.T, logPath string) string {
	t.Helper()
	data := dockerLog(t, logPath)
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.HasPrefix(line, "run ") {
			return line
		}
	}
	t.Fatalf("no docker run call in log:\n%s", data)
	return ""
}

func runnerTestProject(t *testing.T) *agentfile.Project {
	t.Helper()
	projectDir := t.TempDir()
	prompt := agentfile.TextSource("say hi")
	return &agentfile.Project{
		ProjectDir:    projectDir,
		AgentfilePath: filepath.Join(projectDir, "agentfile.yaml"),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "test-agent",
				Version: agentfile.DefaultVersion,
			},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5-mini"}},
				Prompt:  &prompt,
			},
		},
	}
}

func writeRunnerTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
