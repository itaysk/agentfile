package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/itaysk/agentfile/internal/agentfile"
)

// A bare alpine base is enough — AGENTFILE_RENDER_ONLY exits before the
// harness exec, so no harness binary is needed to test runtime rendering.
const runtimeTestBaseImage = "alpine:3.20"

func TestRuntimeEnvEndToEnd(t *testing.T) {
	if os.Getenv("AF_INTEGRATION") != "1" {
		t.Skip("set AF_INTEGRATION=1 to run Docker-backed runtime env tests")
	}
	if output, err := exec.Command("docker", "version").CombinedOutput(); err != nil {
		t.Fatalf("docker is required for AF_INTEGRATION=1: %v\n%s", err, string(output))
	}

	secret := `Bearer to"ken\with$spec,ials&more`
	t.Setenv("SEARCH_MCP_AUTH", secret)
	t.Setenv("GITHUB_TOKEN", secret)

	prompt := agentfile.TextSource("say hi")
	project := &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata:   agentfile.Metadata{Name: "runtime-env-test", Version: agentfile.DefaultVersion},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Image: runtimeTestBaseImage, ClaudeCode: &agentfile.ClaudeCodeHarness{}},
				LLM:     agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-haiku-4-5"}},
				Prompt:  &prompt,
				Envs:    []agentfile.Env{{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}}},
				MCPs: []agentfile.MCP{
					{
						Name: "github",
						Stdio: &agentfile.StdioMCP{
							Command: []string{"github-mcp-server"},
							Envs:    []agentfile.Env{{Name: "GITHUB_PERSONAL_ACCESS_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}}},
						},
					},
					{
						Name: "search",
						HTTP: &agentfile.HTTPMCP{
							URL:     "https://example.com/mcp",
							Headers: []agentfile.Header{{Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "SEARCH_MCP_AUTH"}}}},
						},
					},
				},
			},
		},
	}

	tag := "agentfile-runtime-env-test:latest"
	if os.Getenv("AF_KEEP_INTEGRATION_IMAGES") != "1" {
		tag = fmt.Sprintf("agentfile-runtime-env-test:%d", os.Getpid())
		t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", tag).Run() })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var buildLog bytes.Buffer
	if _, err := Build(ctx, Options{Project: project, Tag: tag, Stdout: &buildLog, Stderr: &buildLog}); err != nil {
		t.Fatalf("Build returned error: %v\n%s", err, buildLog.String())
	}

	t.Run("image carries metadata and runtime env labels", func(t *testing.T) {
		output, err := exec.Command("docker", "image", "inspect", "--format", "{{json .Config.Labels}}", tag).Output()
		if err != nil {
			t.Fatalf("docker image inspect: %v", err)
		}
		var labels map[string]string
		if err := json.Unmarshal(output, &labels); err != nil {
			t.Fatalf("unmarshal labels: %v\n%s", err, output)
		}
		var metadata agentfile.Metadata
		if err := json.Unmarshal([]byte(labels[MetadataLabel]), &metadata); err != nil {
			t.Fatalf("unmarshal metadata label: %v", err)
		}
		var runtimeEnv []string
		if err := json.Unmarshal([]byte(labels[RuntimeEnvLabel]), &runtimeEnv); err != nil {
			t.Fatalf("unmarshal runtimeEnv label: %v", err)
		}
		if metadata.Name != project.AgentFile.Metadata.Name || strings.Join(runtimeEnv, ",") != "GITHUB_TOKEN,SEARCH_MCP_AUTH" || labels[HarnessLabel] != "claudecode" {
			t.Fatalf("labels metadata=%#v runtimeEnv=%#v harness=%q, want built metadata, runtime env names, and harness", metadata, runtimeEnv, labels[HarnessLabel])
		}
	})

	t.Run("secret absent from image", func(t *testing.T) {
		saved, err := exec.Command("docker", "save", tag).Output()
		if err != nil {
			t.Fatalf("docker save: %v", err)
		}
		if bytes.Contains(saved, []byte(secret)) {
			t.Fatal("secret found in docker save output")
		}
	})

	t.Run("render-only writes config with secret verbatim", func(t *testing.T) {
		output, err := exec.Command("docker", "run", "--rm",
			"-e", "AGENTFILE_RENDER_ONLY=1",
			"-e", "SEARCH_MCP_AUTH="+secret,
			"-e", "GITHUB_TOKEN="+secret,
			"--entrypoint", "sh", tag,
			"-c", "/agent/entrypoint && cat /agent/agentfile/claudecode/mcp.json").CombinedOutput()
		if err != nil {
			t.Fatalf("docker run: %v\n%s", err, output)
		}
		var config struct {
			MCPServers map[string]struct {
				Env     map[string]string `json:"env"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(output, &config); err != nil {
			t.Fatalf("rendered mcp.json is invalid JSON: %v\n%s", err, output)
		}
		if got := config.MCPServers["search"].Headers["Authorization"]; got != secret {
			t.Fatalf("Authorization = %q, want %q", got, secret)
		}
		if got := config.MCPServers["github"].Env["GITHUB_PERSONAL_ACCESS_TOKEN"]; got != secret {
			t.Fatalf("GITHUB_PERSONAL_ACCESS_TOKEN = %q, want %q", got, secret)
		}
	})

	t.Run("missing variable fails", func(t *testing.T) {
		output, err := exec.Command("docker", "run", "--rm",
			"-e", "AGENTFILE_RENDER_ONLY=1", "-e", "SEARCH_MCP_AUTH=x", tag).CombinedOutput()
		if err == nil {
			t.Fatalf("container succeeded without GITHUB_TOKEN:\n%s", output)
		}
		if !strings.Contains(string(output), "environment variable GITHUB_TOKEN is required") {
			t.Fatalf("output = %q, want required-variable message", output)
		}
	})

	t.Run("empty variable is a value", func(t *testing.T) {
		output, err := exec.Command("docker", "run", "--rm",
			"-e", "AGENTFILE_RENDER_ONLY=1",
			"-e", "SEARCH_MCP_AUTH=x",
			"-e", "GITHUB_TOKEN=",
			"--entrypoint", "sh", tag,
			"-c", "/agent/entrypoint && cat /agent/agentfile/claudecode/mcp.json").CombinedOutput()
		if err != nil {
			t.Fatalf("container failed on empty GITHUB_TOKEN: %v\n%s", err, output)
		}
		var config struct {
			MCPServers map[string]struct {
				Env map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(output, &config); err != nil {
			t.Fatalf("rendered mcp.json is invalid JSON: %v\n%s", err, output)
		}
		if got, ok := config.MCPServers["github"].Env["GITHUB_PERSONAL_ACCESS_TOKEN"]; !ok || got != "" {
			t.Fatalf("GITHUB_PERSONAL_ACCESS_TOKEN = %q (present=%v), want empty value used verbatim", got, ok)
		}
	})
}
