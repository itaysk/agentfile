package runa

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
)

func TestRunExecutesHarnessWithPrivateProfile(t *testing.T) {
	binDir := t.TempDir()
	harnessPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(harnessPath, []byte(`#!/bin/sh
printf 'args=%s\n' "$*"
printf 'home=%s\n' "$CODEX_HOME"
printf 'token=%s\n' "$GH_TOKEN"
printf 'sandbox=%s\n' "${IS_SANDBOX-unset}"
printf 'cwd=%s\n' "$PWD"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_TOKEN", "from-host")
	workspace := t.TempDir()
	physicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, warning bytes.Buffer
	prompt := "invocation prompt"
	code, err := Run(context.Background(), Options{
		BundlePath: runaBundle(t), Mode: harness.ModeOneShot, Workspace: workspace, Prompt: &prompt,
		Stdout: &stdout, Stderr: io.Discard, WarningStderr: &warning,
	})
	if err != nil || code != 0 {
		t.Fatalf("Run = (%d, %v)", code, err)
	}
	for _, want := range []string{"invocation prompt", "token=from-host", "sandbox=unset", "cwd=" + physicalWorkspace} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if !strings.Contains(warning.String(), Warning) {
		t.Fatalf("warning = %q", warning.String())
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "home=") {
			if _, err := os.Stat(strings.TrimPrefix(line, "home=")); !os.IsNotExist(err) {
				t.Fatalf("temporary profile still exists: %v", err)
			}
		}
	}
}

func TestRunPreservesHarnessExitCodeAndFailureStderr(t *testing.T) {
	binDir := t.TempDir()
	harnessPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(harnessPath, []byte("#!/bin/sh\necho failed >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_TOKEN", "test")
	var failure bytes.Buffer
	code, err := Run(context.Background(), Options{
		BundlePath: runaBundle(t), Mode: harness.ModeOneShot,
		Stdout: io.Discard, Stderr: io.Discard, WarningStderr: io.Discard, FailureStderr: &failure,
	})
	if err != nil || code != 7 || failure.String() != "failed\n" {
		t.Fatalf("Run = (%d, %v), failure stderr = %q", code, err, failure.String())
	}
}

func TestInvocationEnvPrecedence(t *testing.T) {
	t.Setenv("FROM_AMBIENT", "ambient")
	t.Setenv("SHARED", "ambient")
	envFile := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(envFile, []byte("FROM_AMBIENT\nSHARED=file\nQUOTED=\"value with spaces\"\nexport EXPORTED='yes'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := InvocationEnv([]string{envFile}, map[string]string{"SHARED": "explicit"})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"FROM_AMBIENT": "ambient", "SHARED": "explicit", "QUOTED": "value with spaces", "EXPORTED": "yes"} {
		if env[key] != want {
			t.Fatalf("%s = %q, want %q", key, env[key], want)
		}
	}
}

func TestRunRequiresInstalledHarness(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "test")
	code, err := Run(context.Background(), Options{BundlePath: runaBundle(t), WarningStderr: io.Discard})
	if code != 1 || err == nil || !strings.Contains(err.Error(), `requires "codex" on PATH`) {
		t.Fatalf("Run = (%d, %v)", code, err)
	}
}

func TestRunUsesTemporaryWorkspace(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nprintf '%s' \"$PWD\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("GITHUB_TOKEN", "test")
	var stdout bytes.Buffer
	code, err := Run(context.Background(), Options{BundlePath: runaBundle(t), Stdout: &stdout, WarningStderr: io.Discard})
	if code != 0 || err != nil {
		t.Fatalf("Run = (%d, %v)", code, err)
	}
	if _, err := os.Stat(stdout.String()); !os.IsNotExist(err) {
		t.Fatalf("temporary workspace still exists: %q (%v)", stdout.String(), err)
	}
}

func runaBundle(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.tar.gz")
	if err := bundle.Build(runaProject(t), path); err != nil {
		t.Fatal(err)
	}
	return path
}

func runaProject(t *testing.T) *agentfile.Project {
	t.Helper()
	prompt := agentfile.TextSource("default")
	return &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata:   agentfile.Metadata{Name: "runa-test", Version: "latest"},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5"}},
				Prompt:  &prompt,
				Envs: []agentfile.Env{{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{
					RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"},
				}}},
			},
		},
	}
}
