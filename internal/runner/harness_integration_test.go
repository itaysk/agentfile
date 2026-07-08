package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func TestHarnessCLIsWithMockLLM(t *testing.T) {
	if os.Getenv("AF_INTEGRATION") != "1" {
		t.Skip("set AF_INTEGRATION=1 to run Docker-backed harness CLI tests")
	}
	repoRoot := integrationRepoRoot(t)
	requireDocker(t)

	tests := []struct {
		name       string
		dockerfile string
		harness    agentfile.Harness
		llm        agentfile.LLM
		model      string
		credential string
		wantPath   string
		wantAuth   string
		env        map[string]string
	}{
		{
			name:       "codex",
			dockerfile: "codex.Dockerfile",
			harness:    agentfile.Harness{Codex: &agentfile.EmptyObject{}},
			llm:        agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5-mini"}},
			model:      "gpt-5-mini",
			credential: "OPENAI_API_KEY",
			wantPath:   "/v1/responses",
			wantAuth:   "authorization",
		},
		{
			name:       "claudecode",
			dockerfile: "claudecode.Dockerfile",
			harness:    agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}},
			llm:        agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-haiku-4-5"}},
			model:      "claude-haiku-4-5",
			credential: "ANTHROPIC_API_KEY",
			wantPath:   "/v1/messages",
			wantAuth:   "x-api-key",
		},
		{
			name:       "pi",
			dockerfile: "pi.Dockerfile",
			harness:    agentfile.Harness{Pi: &agentfile.EmptyObject{}},
			llm:        agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5-mini"}},
			model:      "gpt-5-mini",
			credential: "OPENAI_API_KEY",
			wantPath:   "/v1/chat/completions",
			wantAuth:   "authorization",
			env:        map[string]string{"PI_OFFLINE": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockLLM(t)
			baseImage := buildHarnessBaseImage(t, repoRoot, tt.name, tt.dockerfile)
			prompt := "say hello from " + tt.name
			tag := integrationImageTag(tt.name)
			if !keepIntegrationImages() {
				t.Cleanup(func() { removeDockerImage(tag) })
			}

			env := map[string]string{
				tt.credential: "dummy",
			}
			for key, value := range tt.env {
				env[key] = value
			}
			extraDockerArgs := mockLLMDockerArgs(t, tt.name, tt.model, mock.origin, env)

			devNull, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer devNull.Close()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			code, err := Run(ctx, Options{
				Project:         integrationProject(t, tt.name, baseImage, tt.harness, tt.llm, prompt),
				Tag:             tag,
				Env:             env,
				Stdin:           devNull,
				Stdout:          &stdout,
				Stderr:          &stderr,
				extraDockerArgs: extraDockerArgs,
			})
			if err != nil || code != 0 {
				t.Fatalf("Run = (%d, %v), want success\nstdout:\n%s\nstderr:\n%s", code, err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), dummyLLMResponse) {
				t.Fatalf("stdout = %q, want %q\nstderr:\n%s", stdout.String(), dummyLLMResponse, stderr.String())
			}

			request := mock.request(t, tt.wantPath)
			if got := request.body["model"]; got != tt.model {
				t.Fatalf("model = %#v, want %q\nbody:\n%s", got, tt.model, request.raw)
			}
			if !strings.Contains(request.raw, prompt) {
				t.Fatalf("request body does not contain prompt %q:\n%s", prompt, request.raw)
			}
			auth := request.header.Get(tt.wantAuth)
			if auth == "" {
				t.Fatalf("%s header is missing", tt.wantAuth)
			}
			if !strings.Contains(auth, "dummy") {
				t.Fatalf("%s header = %q, want dummy credential", tt.wantAuth, auth)
			}
			assertProviderShape(t, tt.wantPath, request.body)
		})
	}
}

func mockLLMDockerArgs(t *testing.T, harness, model, baseURL string, env map[string]string) []string {
	t.Helper()
	args := []string{"--add-host", "host.docker.internal:host-gateway"}
	switch harness {
	case "claudecode":
		env["ANTHROPIC_BASE_URL"] = baseURL
	case "codex":
		configDir := t.TempDir()
		config := `model_provider = "agentfile-test"

[model_providers.agentfile-test]
name = "agentfile test mock"
base_url = ` + strconv.Quote(baseURL+"/v1") + `
env_key = "CODEX_API_KEY"
wire_api = "responses"
supports_websockets = false
`
		if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-v", configDir+":/etc/codex:ro")
	case "pi":
		configDir := t.TempDir()
		config := map[string]any{
			"providers": map[string]any{
				"openai": map[string]any{
					"baseUrl": baseURL + "/v1",
					"api":     "openai-completions",
					"apiKey":  "dummy",
					"models": []any{map[string]any{
						"id":            model,
						"reasoning":     false,
						"input":         []string{"text"},
						"contextWindow": 128000,
						"maxTokens":     4096,
						"cost":          map[string]int{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
					}},
				},
			},
		}
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "models.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-v", configDir+":/agent/agentfile/pi/home")
	}
	return args
}

func assertProviderShape(t *testing.T, path string, body map[string]any) {
	t.Helper()
	switch path {
	case "/v1/responses":
		if _, ok := body["input"]; !ok {
			t.Fatalf("responses request missing input: %#v", body)
		}
	case "/v1/messages", "/v1/chat/completions":
		if _, ok := body["messages"]; !ok {
			t.Fatalf("%s request missing messages: %#v", path, body)
		}
	}
}

func integrationProject(t *testing.T, name, image string, harness agentfile.Harness, llm agentfile.LLM, promptText string) *agentfile.Project {
	t.Helper()
	projectDir := t.TempDir()
	version := agentfile.DefaultVersion
	prompt := agentfile.TextSource(promptText)
	harness.Image = image
	return &agentfile.Project{
		ProjectDir:    projectDir,
		AgentfilePath: filepath.Join(projectDir, "agentfile.yaml"),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata: agentfile.Metadata{
				Name:    "integration-" + name,
				Version: &version,
			},
			Spec: agentfile.Spec{
				Harness: harness,
				LLM:     llm,
				Prompt:  &prompt,
			},
		},
	}
}

func integrationRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func requireDocker(t *testing.T) {
	t.Helper()
	output, err := exec.Command("docker", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("docker is required for AF_INTEGRATION=1: %v\n%s", err, string(output))
	}
}

func buildHarnessBaseImage(t *testing.T, repoRoot, name, dockerfile string) string {
	t.Helper()
	tag := integrationImageTag(name + "-base")
	if keepIntegrationImages() && dockerImageExists(tag) {
		t.Logf("reusing %s", tag)
		return tag
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build", "-f", filepath.Join(repoRoot, "images", dockerfile), "-t", tag, repoRoot)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("build %s base image: %v\n%s", name, err, output.String())
	}
	if !keepIntegrationImages() {
		t.Cleanup(func() { removeDockerImage(tag) })
	}
	return tag
}

func integrationImageTag(name string) string {
	if keepIntegrationImages() {
		return "agentfile-integration-" + name + ":latest"
	}
	return fmt.Sprintf("agentfile-integration-%s:%d", name, os.Getpid())
}

func keepIntegrationImages() bool {
	return os.Getenv("AF_KEEP_INTEGRATION_IMAGES") == "1"
}

func dockerImageExists(tag string) bool {
	return exec.Command("docker", "image", "inspect", tag).Run() == nil
}

func removeDockerImage(tag string) {
	_ = exec.Command("docker", "image", "rm", "-f", tag).Run()
}
