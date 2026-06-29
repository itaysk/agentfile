package runner

import (
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

func TestRunRejectsInvalidWorkspaceHostPathBeforeDocker(t *testing.T) {
	dockerPath, logPath := installFakeDocker(t)
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing"), filePath} {
		project := runnerTestProject(t)
		project.AgentFile.Spec.Workspace.HostBindPath = path

		code, err := Run(context.Background(), Options{
			Project:      project,
			DockerBinary: dockerPath,
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

func installFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	dockerPath := filepath.Join(binDir, "docker")
	writeRunnerTestFile(t, dockerPath, `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"
if [ "$1" = "run" ] && [ -n "${DOCKER_STDIN_LOG:-}" ]; then
  cat > "$DOCKER_STDIN_LOG"
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
