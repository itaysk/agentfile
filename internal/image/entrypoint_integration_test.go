package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
)

// A bare alpine base is enough; the smoke test installs a no-op harness binary.
const entrypointTestBaseImage = "alpine:3.20"

func TestImageSmoke(t *testing.T) {
	if os.Getenv("AF_INTEGRATION") != "1" {
		t.Skip("set AF_INTEGRATION=1 to run the Docker image smoke test")
	}
	if output, err := exec.Command("docker", "version").CombinedOutput(); err != nil {
		t.Fatalf("docker is required for AF_INTEGRATION=1: %v\n%s", err, string(output))
	}

	secret := `Bearer to"ken\with$spec,ials&more`
	literal := "line 1\nline 2"
	t.Setenv("SEARCH_MCP_AUTH", secret)
	t.Setenv("GITHUB_TOKEN", secret)

	prompt := agentfile.TextSource("say hi")
	project := &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata:   agentfile.Metadata{Name: "entrypoint-test", Version: agentfile.DefaultVersion},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}},
				LLM:     agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-haiku-4-5"}},
				Prompt:  &prompt,
				Envs: []agentfile.Env{
					{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"}}},
					{Name: "MULTILINE", ValueSource: agentfile.ValueSource{Value: &literal}},
				},
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

	archivePath := filepath.Join(t.TempDir(), "agent.tar.gz")
	if err := bundle.Build(project, archivePath); err != nil {
		t.Fatal(err)
	}
	tag := "agentfile-entrypoint-test:latest"
	if os.Getenv("AF_KEEP_INTEGRATION_IMAGES") != "1" {
		tag = fmt.Sprintf("agentfile-entrypoint-test:%d", os.Getpid())
		t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", tag).Run() })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var buildLog bytes.Buffer
	if _, err := Build(ctx, Options{BundlePath: archivePath, BaseImage: entrypointTestBaseImage, Tag: tag, Stdout: &buildLog, Stderr: &buildLog}); err != nil {
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
		var metadata bundle.Agent
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
		if !strings.HasPrefix(labels[BundleDigestLabel], "sha256:") {
			t.Fatalf("bundle digest label = %q", labels[BundleDigestLabel])
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

	t.Run("literal image environment preserves value", func(t *testing.T) {
		output, err := exec.Command("docker", "run", "--rm", "--entrypoint", "sh", tag, "-c", `printf '%s' "$MULTILINE"`).Output()
		if err != nil {
			t.Fatal(err)
		}
		if string(output) != literal {
			t.Fatalf("MULTILINE = %q, want %q", output, literal)
		}
	})

	t.Run("entrypoint writes config with secret verbatim", func(t *testing.T) {
		output, err := exec.Command("docker", "run", "--rm",
			"-e", "SEARCH_MCP_AUTH="+secret,
			"-e", "GITHUB_TOKEN="+secret,
			"--entrypoint", "sh", tag,
			"-c", "printf '#!/bin/sh\\n' > /usr/local/bin/claude && chmod +x /usr/local/bin/claude && /agent/entrypoint && cat /agent/profile/claudecode/mcp.json").CombinedOutput()
		if err != nil {
			t.Fatalf("docker run: %v\n%s", err, output)
		}
		type renderedConfig struct {
			MCPServers map[string]struct {
				Env     map[string]string `json:"env"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		var config renderedConfig
		if err := json.Unmarshal(output, &config); err != nil {
			t.Fatalf("rendered mcp.json is invalid JSON: %v\n%s", err, output)
		}
		if got := config.MCPServers["search"].Headers["Authorization"]; got != secret {
			t.Fatalf("Authorization = %q, want %q", got, secret)
		}
		if got := config.MCPServers["github"].Env["GITHUB_PERSONAL_ACCESS_TOKEN"]; got != secret {
			t.Fatalf("GITHUB_PERSONAL_ACCESS_TOKEN = %q, want %q", got, secret)
		}

		bundleRoot := filepath.Join(t.TempDir(), "bundle")
		unpacked, err := bundle.Extract(archivePath, bundleRoot)
		if err != nil {
			t.Fatal(err)
		}
		profileRoot := filepath.Join(t.TempDir(), "profile")
		if _, err := harness.Prepare(unpacked, profileRoot, harness.Invocation{
			Mode: harness.ModeOneShot, Workspace: t.TempDir(),
			Env: map[string]string{"SEARCH_MCP_AUTH": secret, "GITHUB_TOKEN": secret},
		}); err != nil {
			t.Fatal(err)
		}
		hostOutput, err := os.ReadFile(filepath.Join(profileRoot, "claudecode", "mcp.json"))
		if err != nil {
			t.Fatal(err)
		}
		var hostConfig renderedConfig
		if err := json.Unmarshal(hostOutput, &hostConfig); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(hostConfig, config) {
			t.Fatalf("host config %#v != container config %#v", hostConfig, config)
		}
	})
}
