package agentfile

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Resolver struct {
	projectDir string
	tempDir    string
	httpClient *http.Client
}

type ResolvedAssets struct {
	Prompt          string
	HasPrompt       bool
	SystemPrompt    string
	HasSystemPrompt bool
	Skills          []ResolvedSkill
}

type ResolvedSkill struct {
	Name string
	Dir  string
}

func NewResolver(projectDir string) (*Resolver, error) {
	tempDir, err := os.MkdirTemp("", "agentfile-sources-*")
	if err != nil {
		return nil, err
	}
	return &Resolver{
		projectDir: projectDir,
		tempDir:    tempDir,
		httpClient: http.DefaultClient,
	}, nil
}

func (r *Resolver) Close() error {
	if r.tempDir == "" {
		return nil
	}
	return os.RemoveAll(r.tempDir)
}

func (r *Resolver) ResolveProject(p *Project) (*ResolvedAssets, error) {
	assets := &ResolvedAssets{}
	if p.AgentFile.Spec.Prompt != nil {
		content, err := r.ResolveFile(*p.AgentFile.Spec.Prompt)
		if err != nil {
			return nil, fmt.Errorf("resolve spec.prompt: %w", err)
		}
		assets.Prompt = string(content)
		assets.HasPrompt = true
	}
	if p.AgentFile.Spec.SystemPrompt != nil {
		content, err := r.ResolveFile(*p.AgentFile.Spec.SystemPrompt)
		if err != nil {
			return nil, fmt.Errorf("resolve spec.systemPrompt: %w", err)
		}
		assets.SystemPrompt = string(content)
		assets.HasSystemPrompt = true
	}

	seenSkills := map[string]struct{}{}
	for i, source := range p.AgentFile.Spec.Skills {
		dir, err := r.ResolveDirectory(source)
		if err != nil {
			return nil, fmt.Errorf("resolve spec.skills[%d]: %w", i, err)
		}
		name, err := SkillName(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve spec.skills[%d]: %w", i, err)
		}
		if _, ok := seenSkills[name]; ok {
			return nil, fmt.Errorf("spec.skills[%d] resolves to skill name %q, which must be unique within spec.skills", i, name)
		}
		seenSkills[name] = struct{}{}
		assets.Skills = append(assets.Skills, ResolvedSkill{Name: name, Dir: dir})
	}
	return assets, nil
}

func (r *Resolver) ResolveFile(source Source) ([]byte, error) {
	switch {
	case source.Text != nil:
		return []byte(*source.Text), nil
	case source.FS != nil:
		return readRegularFile(r.filesystemPath(*source.FS))
	case source.Git != nil:
		resolvedPath, err := r.resolveGit(*source.Git)
		if err != nil {
			return nil, err
		}
		return readRegularFile(resolvedPath)
	case source.HTTP != nil:
		if source.HTTP.Archive {
			return nil, fmt.Errorf("archive HTTP source resolves to a directory, not a file")
		}
		return r.fetchHTTP(source.HTTP.URL)
	default:
		return nil, fmt.Errorf("source type is missing")
	}
}

func (r *Resolver) ResolveDirectory(source Source) (string, error) {
	switch {
	case source.Text != nil:
		return "", fmt.Errorf("text source does not resolve to a directory")
	case source.FS != nil:
		return requireDir(r.filesystemPath(*source.FS))
	case source.Git != nil:
		resolvedPath, err := r.resolveGit(*source.Git)
		if err != nil {
			return "", err
		}
		return requireDir(resolvedPath)
	case source.HTTP != nil:
		if !source.HTTP.Archive {
			return "", fmt.Errorf("non-archive HTTP source resolves to a file, not a directory")
		}
		return r.fetchAndExtractHTTP(source.HTTP.URL)
	default:
		return "", fmt.Errorf("source type is missing")
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a file", path)
	}
	return os.ReadFile(path)
}

func requireDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return path, nil
}

func (r *Resolver) filesystemPath(source FilesystemSource) string {
	if source.AbsolutePath != "" {
		return source.AbsolutePath
	}
	return filepath.Join(r.projectDir, filepath.FromSlash(source.Path))
}

func (r *Resolver) resolveGit(source GitSource) (string, error) {
	repoURL, inRepoPath := splitGitURL(source.URL)
	cloneDir, err := os.MkdirTemp(r.tempDir, "git-*")
	if err != nil {
		return "", err
	}
	runGit := func(action string, args ...string) error {
		cmd := exec.Command("git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s failed: %w: %s", action, err, strings.TrimSpace(string(output)))
		}
		return nil
	}

	args := []string{"clone", "--quiet"}
	if source.Commit != "" {
		args = append(args, "--depth", "1", "--no-checkout", repoURL, cloneDir)
		shallowOK := false
		if err := runGit("clone", args...); err == nil {
			shallowOK = runGit("fetch", "-C", cloneDir, "fetch", "--quiet", "--depth", "1", "origin", source.Commit) == nil
		}
		if !shallowOK {
			if err := os.RemoveAll(cloneDir); err != nil {
				return "", err
			}
			if err := runGit("clone", "clone", "--quiet", repoURL, cloneDir); err != nil {
				return "", err
			}
		}
		if err := runGit("checkout", "-C", cloneDir, "checkout", "--quiet", source.Commit); err != nil {
			return "", err
		}
	} else {
		args = append(args, "--depth", "1")
		if source.Ref != "" {
			args = append(args, "--branch", source.Ref)
		}
		args = append(args, repoURL, cloneDir)
		if err := runGit("clone", args...); err != nil {
			return "", err
		}
	}

	if inRepoPath == "" {
		return cloneDir, nil
	}
	return filepath.Join(cloneDir, filepath.FromSlash(inRepoPath)), nil
}

func splitGitURL(raw string) (repoURL, inRepoPath string) {
	schemeIndex := strings.Index(raw, "://")
	lastSeparator := strings.LastIndex(raw, "//")
	if schemeIndex >= 0 && lastSeparator > schemeIndex+2 {
		return raw[:lastSeparator], strings.TrimPrefix(raw[lastSeparator+2:], "/")
	}
	return raw, ""
}

func (r *Resolver) fetchHTTP(rawURL string) ([]byte, error) {
	response, err := r.httpClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d from %s", response.StatusCode, rawURL)
	}
	return io.ReadAll(response.Body)
}

func (r *Resolver) fetchAndExtractHTTP(rawURL string) (string, error) {
	data, err := r.fetchHTTP(rawURL)
	if err != nil {
		return "", err
	}
	dest, err := os.MkdirTemp(r.tempDir, "http-archive-*")
	if err != nil {
		return "", err
	}
	if err := extractArchive(data, rawURL, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func extractArchive(data []byte, rawURL, dest string) error {
	archivePath := rawURL
	if parsed, err := url.Parse(rawURL); err == nil {
		archivePath = parsed.Path
	}
	lowerPath := strings.ToLower(path.Base(archivePath))
	switch {
	case strings.HasSuffix(lowerPath, ".zip"):
		return extractZip(data, dest)
	case strings.HasSuffix(lowerPath, ".tar.gz"), strings.HasSuffix(lowerPath, ".tgz"):
		return extractGzipTar(data, dest)
	case strings.HasSuffix(lowerPath, ".tar"):
		return extractTar(bytes.NewReader(data), dest)
	}
	switch {
	case hasZipMagic(data):
		return extractZip(data, dest)
	case hasGzipMagic(data):
		return extractGzipTar(data, dest)
	default:
		return fmt.Errorf("unsupported archive format for %s", rawURL)
	}
}

func extractGzipTar(data []byte, dest string) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer reader.Close()
	return extractTar(reader, dest)
}

func hasZipMagic(data []byte) bool {
	return bytes.HasPrefix(data, []byte("PK\x03\x04")) ||
		bytes.HasPrefix(data, []byte("PK\x05\x06")) ||
		bytes.HasPrefix(data, []byte("PK\x07\x08"))
}

func hasGzipMagic(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func extractZip(data []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		target, err := safeJoin(dest, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		if err := writeFileFromReader(target, src, file.Mode()); err != nil {
			src.Close()
			return err
		}
		if err := src.Close(); err != nil {
			return err
		}
	}
	return nil
}

func extractTar(reader io.Reader, dest string) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFileFromReader(target, tarReader, os.FileMode(header.Mode)); err != nil {
				return err
			}
		default:
			continue
		}
	}
}

func safeJoin(root, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	target := filepath.Join(root, cleanName)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("archive path escapes destination: %s", name)
	}
	return target, nil
}

func writeFileFromReader(path string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, reader); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func SkillName(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	frontMatter, ok := extractFrontMatter(data)
	if !ok {
		return "", fmt.Errorf("SKILL.md must start with YAML front matter containing name")
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(frontMatter, &metadata); err != nil {
		return "", fmt.Errorf("parse SKILL.md front matter: %w", err)
	}
	if err := validatePathSegment(metadata.Name, "SKILL.md front matter name"); err != nil {
		return "", err
	}
	return metadata.Name, nil
}

func extractFrontMatter(data []byte) ([]byte, bool) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, false
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false
	}
	return []byte(rest[:end]), true
}
