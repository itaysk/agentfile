package agentfile

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractArchiveSniffsZipWithoutHelpfulURLSuffix(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	file, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("---\nname: zipped\n---\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractArchive(data.Bytes(), "https://example.com/download?id=1", dest); err != nil {
		t.Fatalf("extractArchive returned error: %v", err)
	}
	if !regularFileExists(filepath.Join(dest, "SKILL.md")) {
		t.Fatal("SKILL.md was not extracted")
	}
}

func TestExtractArchiveSniffsGzipWithoutHelpfulURLSuffix(t *testing.T) {
	var data bytes.Buffer
	gzipWriter := gzip.NewWriter(&data)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0o644, Size: int64(len("---\nname: gzipped\n---\n"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("---\nname: gzipped\n---\n")); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractArchive(data.Bytes(), "https://example.com/download?id=1", dest); err != nil {
		t.Fatalf("extractArchive returned error: %v", err)
	}
	if !regularFileExists(filepath.Join(dest, "SKILL.md")) {
		t.Fatal("SKILL.md was not extracted")
	}
}

func TestResolveGitUsesShallowCloneForDefaultBranchAndRef(t *testing.T) {
	logPath := installFakeGit(t)
	resolver, err := NewResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()

	if _, err := resolver.resolveGit(GitSource{URL: "https://example.com/repo.git"}); err != nil {
		t.Fatalf("resolveGit returned error: %v", err)
	}
	if _, err := resolver.resolveGit(GitSource{URL: "https://example.com/repo.git//skills/greet", Ref: "main"}); err != nil {
		t.Fatalf("resolveGit returned error: %v", err)
	}

	lines := readLogLines(t, logPath)
	if len(lines) != 2 {
		t.Fatalf("git calls = %#v, want 2 calls", lines)
	}
	if !strings.Contains(lines[0], "clone --quiet --depth 1 https://example.com/repo.git") {
		t.Fatalf("default clone args = %q, want shallow clone", lines[0])
	}
	if !strings.Contains(lines[1], "clone --quiet --depth 1 --branch main https://example.com/repo.git") {
		t.Fatalf("ref clone args = %q, want shallow branch clone", lines[1])
	}
}

func TestResolveGitUsesShallowFetchForCommit(t *testing.T) {
	logPath := installFakeGit(t)
	resolver, err := NewResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()

	if _, err := resolver.resolveGit(GitSource{URL: "https://example.com/repo.git", Commit: "abc123"}); err != nil {
		t.Fatalf("resolveGit returned error: %v", err)
	}

	lines := readLogLines(t, logPath)
	if len(lines) != 3 {
		t.Fatalf("git calls = %#v, want clone, fetch, and checkout", lines)
	}
	if !strings.Contains(lines[0], "clone --quiet --depth 1 --no-checkout https://example.com/repo.git") {
		t.Fatalf("commit clone args = %q, want shallow no-checkout clone", lines[0])
	}
	if !strings.Contains(lines[1], "fetch --quiet --depth 1 origin abc123") {
		t.Fatalf("commit fetch args = %q, want shallow commit fetch", lines[1])
	}
	if !strings.Contains(lines[2], "checkout --quiet abc123") {
		t.Fatalf("checkout args = %q, want commit checkout", lines[2])
	}
}

func TestResolveGitFallsBackToFullCloneWhenCommitFetchFails(t *testing.T) {
	logPath := installFakeGit(t)
	t.Setenv("FAIL_GIT_FETCH", "1")
	resolver, err := NewResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()

	if _, err := resolver.resolveGit(GitSource{URL: "https://example.com/repo.git", Commit: "abc123"}); err != nil {
		t.Fatalf("resolveGit returned error: %v", err)
	}

	lines := readLogLines(t, logPath)
	if len(lines) != 4 {
		t.Fatalf("git calls = %#v, want shallow clone, fetch, full clone, and checkout", lines)
	}
	if !strings.Contains(lines[0], "clone --quiet --depth 1 --no-checkout https://example.com/repo.git") {
		t.Fatalf("commit clone args = %q, want shallow no-checkout clone", lines[0])
	}
	if !strings.Contains(lines[1], "fetch --quiet --depth 1 origin abc123") {
		t.Fatalf("commit fetch args = %q, want shallow commit fetch", lines[1])
	}
	if !strings.Contains(lines[2], "clone --quiet https://example.com/repo.git") || strings.Contains(lines[2], "--depth") {
		t.Fatalf("fallback clone args = %q, want full clone", lines[2])
	}
	if !strings.Contains(lines[3], "checkout --quiet abc123") {
		t.Fatalf("checkout args = %q, want commit checkout", lines[3])
	}
}

func TestSkillNameRejectsUnsafePathSegments(t *testing.T) {
	tests := []string{
		`"."`,
		`".."`,
		`nested/name`,
		`nested\name`,
	}
	for _, yamlName := range tests {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+yamlName+"\n---\n")

		_, err := SkillName(dir)
		if err == nil {
			t.Fatalf("SkillName accepted unsafe name %s", yamlName)
		}
		if !strings.Contains(err.Error(), "single path segment") {
			t.Fatalf("error = %q, want single path segment detail", err)
		}
	}
}

func installFakeGit(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	gitPath := filepath.Join(binDir, "git")
	writeTestFile(t, gitPath, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GIT_ARGS_LOG\"\nif [ \"$FAIL_GIT_FETCH\" = \"1\" ]; then\n  for arg do\n    if [ \"$arg\" = \"fetch\" ]; then\n      echo fetch failed >&2\n      exit 1\n    fi\n  done\nfi\nexit 0\n")
	if err := os.Chmod(gitPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_ARGS_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func readLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}
