package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func TestBuildIsReproducibleAndPortable(t *testing.T) {
	project := testProject(t)
	first := filepath.Join(t.TempDir(), "first.tar.gz")
	second := filepath.Join(t.TempDir(), "second.tar.gz")
	t.Setenv("GITHUB_TOKEN", "must-not-enter-the-bundle")
	if err := Build(project, first); err != nil {
		t.Fatal(err)
	}
	if err := Build(project, second); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(first)
	secondBytes, _ := os.ReadFile(second)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("identical projects produced different archive bytes")
	}
	if bytes.Contains(firstBytes, []byte("must-not-enter-the-bundle")) {
		t.Fatal("ambient runtime secret entered the bundle")
	}
	extracted := filepath.Join(t.TempDir(), "bundle")
	unpacked, err := Extract(first, extracted)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(firstBytes)
	if unpacked.Root != extracted {
		t.Fatalf("unpacked root = %q, want %q", unpacked.Root, extracted)
	}
	if unpacked.Digest != "sha256:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("digest = %q, want archive digest", unpacked.Digest)
	}
	if unpacked.Manifest.BundleVersion != Version ||
		unpacked.Manifest.Harness != "codex" ||
		unpacked.Manifest.Assets.Prompt != "prompt.md" ||
		unpacked.Manifest.Environment.Mappings["GH_TOKEN"] != "GITHUB_TOKEN" {
		t.Fatalf("manifest = %#v", unpacked.Manifest)
	}
	manifestJSON := readArchiveEntry(t, first, "manifest.json")
	if strings.Contains(manifestJSON, project.ProjectDir) || strings.Contains(manifestJSON, "absolutePath") {
		t.Fatalf("manifest contains build-machine path: %s", manifestJSON)
	}
	for _, unwanted := range []string{`"apiVersion"`, `"kind"`, `"spec"`, `"mcps"`, `"fs"`} {
		if strings.Contains(manifestJSON, unwanted) {
			t.Fatalf("manifest contains source-agentfile field %s: %s", unwanted, manifestJSON)
		}
	}

	mode, err := os.Stat(filepath.Join(extracted, "skills", "demo", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode()&0o111 == 0 {
		t.Fatal("skill executable bit was not preserved")
	}
	if _, err := os.Stat(filepath.Join(extracted, "harness", "codex", "config.toml.tmpl")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header tar.Header
		want   string
	}{
		{"absolute", tar.Header{Name: "/etc/passwd", Typeflag: tar.TypeReg}, "invalid bundle path"},
		{"traversal", tar.Header{Name: "../outside", Typeflag: tar.TypeReg}, "invalid bundle path"},
		{"backslash", tar.Header{Name: `dir\\file`, Typeflag: tar.TypeReg}, "invalid bundle path"},
		{"windows absolute", tar.Header{Name: `C:/outside`, Typeflag: tar.TypeReg}, "invalid bundle path"},
		{"symlink", tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target"}, "unsupported type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "unsafe.tar.gz")
			writeTar(t, archivePath, tt.header)
			if _, err := Extract(archivePath, filepath.Join(t.TempDir(), "bundle")); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Extract error = %v, want %q", err, tt.want)
			}
		})
	}
	if err := validateHeader(&tar.Header{Name: "file/", Typeflag: tar.TypeReg}); err == nil || !strings.Contains(err.Error(), "invalid bundle path") {
		t.Fatalf("validateHeader error = %v, want invalid regular-file path with trailing slash", err)
	}
}

func TestExtractRejectsDuplicatePathsAndNonemptyDestination(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "duplicate.tar.gz")
	header := tar.Header{Name: "same", Typeflag: tar.TypeReg}
	writeTar(t, archivePath, header, header)
	if _, err := Extract(archivePath, filepath.Join(t.TempDir(), "bundle")); err == nil || !strings.Contains(err.Error(), "duplicate path") {
		t.Fatalf("Extract error = %v", err)
	}
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "existing"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(archivePath, dest); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Fatalf("Extract error = %v", err)
	}
}

func TestExtractHashesAndValidatesEntireArchive(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := Build(testProject(t), archivePath); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(archivePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	unpacked, err := Extract(archivePath, filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(archive)
	if unpacked.Digest != "sha256:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("digest = %q, want complete archive digest", unpacked.Digest)
	}

	file, err = os.OpenFile(archivePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("invalid gzip tail"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(archivePath, filepath.Join(t.TempDir(), "invalid")); err == nil || !strings.Contains(err.Error(), "read bundle gzip") {
		t.Fatalf("Extract error = %v, want invalid gzip tail", err)
	}
}

func TestExtractRejectsOversizedArchive(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "large.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxArchiveSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(archivePath, filepath.Join(t.TempDir(), "bundle")); err == nil || !strings.Contains(err.Error(), "archive bytes") {
		t.Fatalf("Extract error = %v, want archive size limit", err)
	}
}

func TestWriteLayoutKeepsRuntimeValuesOutOfClaudeTemplate(t *testing.T) {
	runtimeName := "MCP_TOKEN"
	literal := "fixed"
	t.Setenv(runtimeName, "must-not-enter-template")
	project := testProject(t)
	project.AgentFile.Spec.Harness = agentfile.Harness{ClaudeCode: &agentfile.ClaudeCodeHarness{}}
	project.AgentFile.Spec.LLM = agentfile.LLM{Anthropic: &agentfile.ModelProvider{Model: "claude-model"}}
	project.AgentFile.Spec.MCPs = []agentfile.MCP{
		{Name: "local", Stdio: &agentfile.StdioMCP{Command: []string{"server", "--stdio"}, Envs: []agentfile.Env{{Name: "MODE", ValueSource: agentfile.ValueSource{Value: &literal}}}}},
		{Name: "remote", HTTP: &agentfile.HTTPMCP{URL: "https://example.com", Headers: []agentfile.Header{{Name: "Authorization", ValueSource: agentfile.ValueSource{RuntimeEnv: &agentfile.RuntimeEnvSource{Name: runtimeName}}}}}},
	}
	root := t.TempDir()
	_, err := WriteLayout(root, project, &agentfile.ResolvedAssets{
		Prompt: "review this", HasPrompt: true,
		Skills: []agentfile.ResolvedSkill{{Name: "demo", Dir: filepath.Join(project.ProjectDir, "skill")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	template, err := os.ReadFile(filepath.Join(root, "harness", "claudecode", "mcp.json.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"server", "--stdio", "fixed", RefTokenPrefix + runtimeName + "__"} {
		if !strings.Contains(string(template), want) {
			t.Fatalf("template = %s, want %q", template, want)
		}
	}
	if bytes.Contains(template, []byte("must-not-enter-template")) {
		t.Fatalf("runtime secret entered template: %s", template)
	}
}

func TestValidateManifestAndUnpacked(t *testing.T) {
	project := testProject(t)
	root := t.TempDir()
	manifest, err := WriteLayout(root, project, &agentfile.ResolvedAssets{
		Prompt: "review this", HasPrompt: true,
		Skills: []agentfile.ResolvedSkill{{Name: "demo", Dir: filepath.Join(project.ProjectDir, "skill")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"version", func(m *Manifest) { m.BundleVersion = "agentfile.build/bundle/v2" }, "unsupported bundle version"},
		{"harness", func(m *Manifest) { m.Harness = "future" }, "unsupported harness"},
		{"prompt path", func(m *Manifest) { m.Assets.Prompt = "../prompt.md" }, "clean relative slash path"},
		{"absolute system prompt", func(m *Manifest) { m.Assets.SystemPrompt = "/system-prompt.md" }, "clean relative slash path"},
		{"skill layout", func(m *Manifest) {
			m.Assets.Skills = []string{"other/demo"}
		}, "must match skills/<name>"},
		{"nested skill", func(m *Manifest) {
			m.Assets.Skills = []string{"skills/team/demo"}
		}, "must match skills/<name>"},
		{"duplicate skill", func(m *Manifest) {
			m.Assets.Skills = []string{"skills/demo", "skills/demo"}
		}, "duplicate skill name"},
		{"config env", func(m *Manifest) {
			m.Assets.ConfigEnv = []string{"NOT-AN-ENV"}
		}, "must match"},
		{"config env order", func(m *Manifest) {
			m.Assets.ConfigEnv = []string{"ZED", "ALPHA"}
		}, "must be sorted"},
		{"mapping source", func(m *Manifest) {
			m.Environment.Mappings = map[string]string{"TOKEN": "NOT-AN-ENV"}
		}, "must match"},
		{"bare oauth", func(m *Manifest) {
			m.Harness = "claudecode"
			m.Model.Provider = "anthropic"
			m.Bare = true
			m.Assets.Skills = nil
			m.Environment.Defaults = map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "token"}
			m.Environment.Mappings = nil
		}, "cannot declare CLAUDE_CODE_OAUTH_TOKEN"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			copy := *manifest
			tt.mutate(&copy)
			if err := validateManifest(&copy); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateManifest error = %v, want %q", err, tt.want)
			}
		})
	}
	unpacked := &Unpacked{Root: root, Manifest: *manifest}
	if err := validateUnpacked(unpacked); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, manifest.Assets.Prompt)); err != nil {
		t.Fatal(err)
	}
	if err := validateUnpacked(unpacked); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("validateUnpacked error = %v", err)
	}
}

func TestDefaultFilenameSanitizesMetadata(t *testing.T) {
	got := DefaultFilename(agentfile.Metadata{Name: `team/reviewer`, Version: `v1\beta`})
	if got != "team-reviewer-v1-beta.tar.gz" {
		t.Fatalf("DefaultFilename = %q", got)
	}
}

func testProject(t *testing.T) *agentfile.Project {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skill")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := agentfile.TextSource("review this")
	return &agentfile.Project{
		ProjectDir: dir,
		AgentFile: agentfile.AgentFile{
			APIVersion: agentfile.APIVersion,
			Kind:       agentfile.Kind,
			Metadata:   agentfile.Metadata{Name: "reviewer", Version: "1.0"},
			Spec: agentfile.Spec{
				Harness: agentfile.Harness{Codex: &agentfile.EmptyObject{}},
				LLM:     agentfile.LLM{OpenAI: &agentfile.ModelProvider{Model: "gpt-5"}},
				Prompt:  &prompt,
				Skills:  []agentfile.Source{{FS: &agentfile.FilesystemSource{Path: "skill"}}},
				Envs: []agentfile.Env{{Name: "GH_TOKEN", ValueSource: agentfile.ValueSource{
					RuntimeEnv: &agentfile.RuntimeEnvSource{Name: "GITHUB_TOKEN"},
				}}},
			},
		},
	}
}

func readArchiveEntry(t *testing.T, archivePath, name string) string {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == name {
			var data bytes.Buffer
			if _, err := data.ReadFrom(reader); err != nil {
				t.Fatal(err)
			}
			return data.String()
		}
	}
}

func writeTar(t *testing.T, path string, headers ...tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	for i := range headers {
		if err := writer.WriteHeader(&headers[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
