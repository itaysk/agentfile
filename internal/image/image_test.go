package image

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
)

func TestWriteBuildContextUsesOnlyBundleInputs(t *testing.T) {
	project := imageTestProject(t)
	bundlePath := filepath.Join(t.TempDir(), "agent.tar.gz")
	if err := bundle.Build(project, bundlePath); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(project.ProjectDir); err != nil {
		t.Fatal(err)
	}
	contextDir := t.TempDir()
	unpacked, err := WriteBuildContext(contextDir, bundlePath, "example/base:1")
	if err != nil {
		t.Fatal(err)
	}
	if unpacked.Manifest.Agent.Name != "image-test" || !strings.HasPrefix(unpacked.Digest, "sha256:") {
		t.Fatalf("unpacked bundle = %#v", unpacked)
	}
	entries, err := os.ReadDir(contextDir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "Dockerfile,bundle,entrypoint" {
		t.Fatalf("context entries = %#v", names)
	}
	dockerfile, _ := os.ReadFile(filepath.Join(contextDir, "Dockerfile"))
	for _, want := range []string{"FROM example/base:1", "ARG AGENTFILE_LITERAL_LOG_LEVEL", "ENV LOG_LEVEL=${AGENTFILE_LITERAL_LOG_LEVEL}", "COPY bundle /agent/bundle"} {
		if !strings.Contains(string(dockerfile), want) {
			t.Fatalf("Dockerfile = %s, want %q", dockerfile, want)
		}
	}
	if strings.Contains(string(dockerfile), "SECRET") {
		t.Fatalf("Dockerfile contains runtime variable value/source: %s", dockerfile)
	}
	entrypoint, _ := os.ReadFile(filepath.Join(contextDir, "entrypoint"))
	if strings.Contains(string(entrypoint), "cp -R /agent/bundle/.") || !strings.Contains(string(entrypoint), "/agent/profile/codex/home/.codex") {
		t.Fatalf("entrypoint does not keep bundle assets separate from the writable profile: %s", entrypoint)
	}

	defaultContext := t.TempDir()
	if _, err := WriteBuildContext(defaultContext, bundlePath, ""); err != nil {
		t.Fatal(err)
	}
	defaultDockerfile, _ := os.ReadFile(filepath.Join(defaultContext, "Dockerfile"))
	if !strings.Contains(string(defaultDockerfile), "FROM itaysk/codex:latest") {
		t.Fatalf("Dockerfile = %s, want codex default base image", defaultDockerfile)
	}

	if _, err := WriteBuildContext(t.TempDir(), bundlePath, "bad image"); err == nil || !strings.Contains(err.Error(), "without whitespace") {
		t.Fatalf("WriteBuildContext error = %v, want invalid base image", err)
	}
}

func TestBuildInvokesDockerWithBundleMetadata(t *testing.T) {
	project := imageTestProject(t)
	archivePath := filepath.Join(t.TempDir(), "agent.tar.gz")
	if err := bundle.Build(project, archivePath); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "docker.log")
	docker := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DOCKER_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_LOG", logPath)
	tag, err := Build(context.Background(), Options{BundlePath: archivePath, BaseImage: "example/base:1", DockerBinary: docker})
	if err != nil {
		t.Fatal(err)
	}
	if tag != "image-test:1" {
		t.Fatalf("tag = %q", tag)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"build\n-t\nimage-test:1\n",
		MetadataLabel + `={"name":"image-test","version":"1"}`,
		RuntimeEnvLabel + `=["SECRET"]`,
		HarnessLabel + "=codex",
		BundleDigestLabel + "=sha256:",
		"--build-arg\nAGENTFILE_LITERAL_LOG_LEVEL=info\n",
	} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("docker args = %q, want %q", args, want)
		}
	}
}

func TestEntrypointScriptSerializesHarnessCommands(t *testing.T) {
	literal := "default"
	for _, tt := range []struct {
		name    string
		harness agentfile.Harness
		llm     agentfile.LLM
	}{
		{"claude", agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}, agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-model"}}},
		{"codex", agentfile.Harness{Codex: &agentfile.EmptyObject{}}, agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-model"}}},
		{"pi", agentfile.Harness{Pi: &agentfile.EmptyObject{}}, agentfile.LLM{OpenRouter: &agentfile.ModelProvider{Model: "provider/model"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			project := imageTestProject(t)
			project.AgentFile.Spec.Harness = tt.harness
			project.AgentFile.Spec.LLM = tt.llm
			project.AgentFile.Spec.Envs = []agentfile.Env{
				{Name: "LITERAL", ValueSource: agentfile.ValueSource{Value: &literal}},
				{Name: "TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "SOURCE_TOKEN"}}},
			}
			if tt.harness.Pi == nil {
				project.AgentFile.Spec.MCPs = []agentfile.MCP{{
					Name: "remote",
					HTTP: &agentfile.HTTPMCP{
						URL: "https://example.com",
						Headers: []agentfile.Header{{
							Name: "Authorization",
							ValueSource: agentfile.ValueSource{
								RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "MCP_TOKEN"},
							},
						}},
					},
				}}
			}
			bundleRoot := t.TempDir()
			manifest, err := bundle.WriteLayout(bundleRoot, project, &agentfile.ResolvedAssets{Prompt: "hello", HasPrompt: true})
			if err != nil {
				t.Fatal(err)
			}
			manifest.Assets.Skills = []string{"skills/demo"}
			script, err := EntrypointScript(*manifest)
			if err != nil {
				t.Fatal(err)
			}
			syntax := exec.Command("sh", "-n")
			syntax.Stdin = strings.NewReader(script)
			if output, err := syntax.CombinedOutput(); err != nil {
				t.Fatalf("entrypoint syntax: %v\n%s\n%s", err, output, script)
			}
			for _, mode := range []harness.Mode{harness.ModeTUI, harness.ModeACP, harness.ModeOneShot} {
				command, err := harness.NewCommand(*manifest, harness.CommandOptions{
					Mode: mode, BundleRoot: "/agent/bundle", ProfileRoot: "/agent/profile",
					Workspace: "/agent/workspace", Model: modelShellToken,
					Prompt: promptShellToken, SystemPrompt: systemShellToken,
				})
				if err != nil {
					t.Fatal(err)
				}
				want := "exec " + shellCommand(command)
				if !strings.Contains(script, want) {
					t.Fatalf("entrypoint for %s is missing %q:\n%s", mode, want, script)
				}
			}
			for _, want := range []string{"umask 077", "chmod 700 /agent/profile", "/agent/bundle/prompt.md", "/agent/profile", "environment variable SOURCE_TOKEN is required", "AGENTFILE_RUN_MODE"} {
				if !strings.Contains(script, want) {
					t.Fatalf("entrypoint is missing %q:\n%s", want, script)
				}
			}
			if tt.harness.Pi == nil {
				for _, want := range []string{"/agent/bundle/skills/demo/.", "chmod 600", `$(printf '\r')`} {
					if !strings.Contains(script, want) {
						t.Fatalf("entrypoint is missing idempotent/private profile setup %q:\n%s", want, script)
					}
				}
			}
			if strings.Contains(script, "/agent/agentfile") || strings.Contains(script, "cp -R /agent/bundle/.") {
				t.Fatalf("entrypoint still copies the bundle into a writable invocation root:\n%s", script)
			}
			private, profile, restore, prompt := strings.Index(script, "umask 077"), strings.Index(script, "mkdir -p /agent/profile"), strings.Index(script, `umask "$AGENTFILE_UMASK"`), strings.Index(script, `if [ "$AGENTFILE_RUN_MODE" = oneshot ]`)
			if private < 0 || profile < private || restore < profile || prompt < restore || !strings.Contains(script[restore:prompt], "unset AGENTFILE_UMASK") {
				t.Fatalf("entrypoint does not scope its private umask to profile setup:\n%s", script)
			}
		})
	}
	if got := shQuote("it's"); got != `'it'"'"'s'` {
		t.Fatalf("shQuote = %q", got)
	}
}

func TestManifestMapOutputIsSorted(t *testing.T) {
	dockerfile := dockerfile("example/base", map[string]string{"ZED": "z", "ALPHA": "a"})
	alpha, zed := strings.Index(dockerfile, "AGENTFILE_LITERAL_ALPHA"), strings.Index(dockerfile, "AGENTFILE_LITERAL_ZED")
	if alpha < 0 || zed < 0 || alpha > zed {
		t.Fatalf("Dockerfile defaults are not sorted:\n%s", dockerfile)
	}

	var entrypoint strings.Builder
	appendDeclaredEnv(&entrypoint, bundle.Environment{
		Defaults: map[string]string{"ZED": "z", "ALPHA": "a"},
		Mappings: map[string]string{"ZED_TOKEN": "ZED_SOURCE", "ALPHA_TOKEN": "ALPHA_SOURCE"},
	})
	script := entrypoint.String()
	for _, pair := range [][2]string{
		{"export ALPHA=", "export ZED="},
		{"export ALPHA_TOKEN=", "export ZED_TOKEN="},
	} {
		first, second := strings.Index(script, pair[0]), strings.Index(script, pair[1])
		if first < 0 || second < 0 || first > second {
			t.Fatalf("entrypoint environment is not sorted:\n%s", script)
		}
	}
}

func imageTestProject(t *testing.T) *agentfile.Project {
	t.Helper()
	prompt := agentfile.TextSource("hello")
	literal := "info"
	return &agentfile.Project{
		ProjectDir: t.TempDir(),
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion, Kind: agentfile.Kind,
			Metadata: agentfile.Metadata{Name: "image-test", Version: "1"},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5"}},
				Prompt:  &prompt,
				Envs: []agentfile.Env{
					{Name: "LOG_LEVEL", ValueSource: agentfile.ValueSource{Value: &literal}},
					{Name: "TOKEN", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "SECRET"}}},
				},
			},
		},
	}
}
