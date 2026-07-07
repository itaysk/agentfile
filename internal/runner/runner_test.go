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
	workspace := t.TempDir()

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
	if !strings.Contains(dockerRunArgs(t, logPath), "-v "+workspace+":/agent/workspace") {
		t.Fatalf("docker run args = %q, want workspace mount", dockerRunArgs(t, logPath))
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

	envs := runEnv(project.AgentFile, map[string]string{})
	if got := envs["GITHUB_TOKEN"]; got != "from-host" {
		t.Fatalf("GITHUB_TOKEN = %q, want from-host", got)
	}
	if _, ok := envs["MISSING_ON_HOST"]; ok {
		t.Fatalf("MISSING_ON_HOST forwarded despite being unset on host")
	}

	envs = runEnv(project.AgentFile, map[string]string{"GITHUB_TOKEN": "explicit"})
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

func dockerRunArgs(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "run ") {
			return line
		}
	}
	t.Fatalf("no docker run call in log:\n%s", string(data))
	return ""
}

func runnerTestProject(t *testing.T) *agentfile.Project {
	t.Helper()
	projectDir := t.TempDir()
	version := agentfile.DefaultVersion
	prompt := agentfile.TextSource("say hi")
	return &agentfile.Project{
		ProjectDir:    projectDir,
		AgentfilePath: filepath.Join(projectDir, "agentfile.yaml"),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "test-agent",
				Version: &version,
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
