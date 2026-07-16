package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

func writeArchive(bundleRoot, bundlePath string) error {
	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(bundlePath), ".agentfile-bundle-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	gz := gzip.NewWriter(temp)
	gz.Header.ModTime = time.Unix(0, 0)
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)

	err = fs.WalkDir(os.DirFS(bundleRoot), ".", func(name string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		return writeArchiveEntry(tw, bundleRoot, filepath.Join(bundleRoot, filepath.FromSlash(name)))
	})
	for _, closer := range []io.Closer{tw, gz, temp} {
		if closeErr := closer.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tempName, bundlePath); err != nil {
		return err
	}
	return nil
}

func writeArchiveEntry(writer *tar.Writer, root, filePath string) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return err
	}
	name := filepath.ToSlash(rel)
	header := &tar.Header{
		Name:       name,
		Mode:       0o644,
		ModTime:    time.Unix(0, 0),
		AccessTime: time.Unix(0, 0),
		ChangeTime: time.Unix(0, 0),
		Format:     tar.FormatPAX,
	}
	switch {
	case info.IsDir():
		header.Name += "/"
		header.Typeflag = tar.TypeDir
		header.Mode = 0o755
	case info.Mode().IsRegular():
		header.Typeflag = tar.TypeReg
		header.Size = info.Size()
		if info.Mode()&0o111 != 0 {
			header.Mode = 0o755
		}
	default:
		return fmt.Errorf("bundle entry %s is not a regular file or directory", name)
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	if header.Typeflag == tar.TypeDir {
		return nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

// Extract validates and extracts an agent bundle into bundleRoot.
func Extract(bundlePath, bundleRoot string) (*Unpacked, error) {
	if err := os.MkdirAll(bundleRoot, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("bundle extraction destination %q is not empty", bundleRoot)
	}
	unpacked, err := readArchive(bundlePath, bundleRoot)
	if err != nil {
		return nil, err
	}
	return unpacked, validateUnpacked(unpacked)
}

func readArchive(bundlePath, dest string) (*Unpacked, error) {
	file, err := os.Open(bundlePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxArchiveSize {
		return nil, fmt.Errorf("bundle exceeds %d archive bytes", maxArchiveSize)
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: file, N: maxArchiveSize + 1}
	reader := io.TeeReader(limited, hash)
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("read bundle gzip: %w", err)
	}
	defer gz.Close()
	tarReader := tar.NewReader(gz)
	var manifest Manifest
	found := false
	seen := map[string]struct{}{}
	var total int64
	count := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read bundle: %w", err)
		}
		count++
		if count > maxArchiveEntries {
			return nil, fmt.Errorf("bundle exceeds %d entries", maxArchiveEntries)
		}
		if err := validateHeader(header); err != nil {
			return nil, err
		}
		cleanName := strings.TrimSuffix(header.Name, "/")
		if _, ok := seen[cleanName]; ok {
			return nil, fmt.Errorf("bundle contains duplicate path %q", cleanName)
		}
		seen[cleanName] = struct{}{}
		total += header.Size
		if total > maxExtractedSize {
			return nil, fmt.Errorf("bundle exceeds %d extracted bytes", maxExtractedSize)
		}
		if header.Name == ManifestName {
			found = true
		}
		target := filepath.Join(dest, filepath.FromSlash(cleanName))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		mode := os.FileMode(0o644)
		if header.Mode&0o111 != 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return nil, err
		}
		_, copyErr := io.CopyN(out, tarReader, header.Size)
		closeErr := out.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	_, gzipErr := io.Copy(io.Discard, gz)
	_, drainErr := io.Copy(io.Discard, reader)
	if limited.N == 0 {
		return nil, fmt.Errorf("bundle exceeds %d archive bytes", maxArchiveSize)
	}
	if gzipErr != nil {
		return nil, fmt.Errorf("read bundle gzip: %w", gzipErr)
	}
	if drainErr != nil {
		return nil, fmt.Errorf("read bundle: %w", drainErr)
	}
	if !found {
		return nil, fmt.Errorf("bundle is missing %s", ManifestName)
	}
	manifestData, err := os.ReadFile(filepath.Join(dest, ManifestName))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse bundle manifest: %w", err)
	}
	if err := validateManifest(&manifest); err != nil {
		return nil, err
	}
	return &Unpacked{
		Root:     dest,
		Manifest: manifest,
		Digest:   "sha256:" + hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func validateHeader(header *tar.Header) error {
	name := header.Name
	trimmed := strings.TrimSuffix(name, "/")
	invalidPath := name == "" ||
		trimmed == "." ||
		strings.Contains(name, "\\") ||
		path.IsAbs(name) ||
		isWindowsVolumePath(trimmed) ||
		trimmed == ".." ||
		strings.HasPrefix(trimmed, "../") ||
		path.Clean(trimmed) != trimmed ||
		(strings.HasSuffix(name, "/") && header.Typeflag != tar.TypeDir)
	if invalidPath {
		return fmt.Errorf("invalid bundle path %q", name)
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
		return fmt.Errorf("bundle entry %q has unsupported type %d", name, header.Typeflag)
	}
	if header.Size < 0 || header.Size > maxExtractedSize {
		return fmt.Errorf("bundle entry %q has invalid size", name)
	}
	return nil
}

func validateManifest(manifest *Manifest) error {
	if manifest.BundleVersion != Version {
		return fmt.Errorf("unsupported bundle version %q (want %q)", manifest.BundleVersion, Version)
	}
	if strings.TrimSpace(manifest.Agent.Name) == "" {
		return fmt.Errorf("invalid bundle manifest: agent.name is required")
	}
	if manifest.Agent.Version == "" {
		return fmt.Errorf("invalid bundle manifest: agent.version is required")
	}
	switch manifest.Harness {
	case "claudecode":
		if manifest.Model.Provider != "anthropic" {
			return fmt.Errorf("invalid bundle manifest: claudecode harness requires anthropic provider")
		}
	case "codex":
		if manifest.Model.Provider != "openai" {
			return fmt.Errorf("invalid bundle manifest: codex harness requires openai provider")
		}
	case "pi":
	default:
		return fmt.Errorf("invalid bundle manifest: unsupported harness %q", manifest.Harness)
	}
	if manifest.Bare && manifest.Harness != "claudecode" {
		return fmt.Errorf("invalid bundle manifest: bare requires claudecode harness")
	}
	switch manifest.Model.Provider {
	case "anthropic", "openai", "openrouter":
	default:
		return fmt.Errorf("invalid bundle manifest: unsupported model provider %q", manifest.Model.Provider)
	}
	if manifest.Model.Name == "" {
		return fmt.Errorf("invalid bundle manifest: model.name is required")
	}
	for name, value := range map[string]string{
		"assets.prompt":         manifest.Assets.Prompt,
		"assets.systemPrompt":   manifest.Assets.SystemPrompt,
		"assets.configTemplate": manifest.Assets.ConfigTemplate,
	} {
		if value != "" {
			if err := validateRelativePath(value); err != nil {
				return fmt.Errorf("invalid %s: %w", name, err)
			}
		}
	}
	if manifest.Harness == "codex" && manifest.Assets.ConfigTemplate == "" {
		return fmt.Errorf("invalid bundle manifest: codex harness requires assets.configTemplate")
	}
	if manifest.Harness == "pi" && manifest.Assets.ConfigTemplate != "" {
		return fmt.Errorf("invalid bundle manifest: pi harness does not support assets.configTemplate")
	}
	if manifest.Assets.ConfigTemplate == "" && len(manifest.Assets.ConfigEnv) > 0 {
		return fmt.Errorf("invalid bundle manifest: assets.configEnv requires assets.configTemplate")
	}
	if manifest.Bare && len(manifest.Assets.Skills) > 0 {
		return fmt.Errorf("invalid bundle manifest: bare cannot be used with skills")
	}
	if manifest.Bare {
		_, hasDefault := manifest.Environment.Defaults["CLAUDE_CODE_OAUTH_TOKEN"]
		_, hasMapping := manifest.Environment.Mappings["CLAUDE_CODE_OAUTH_TOKEN"]
		if hasDefault || hasMapping {
			return fmt.Errorf("invalid bundle manifest: bare cannot declare CLAUDE_CODE_OAUTH_TOKEN")
		}
	}
	seenSkills := map[string]struct{}{}
	for i, rel := range manifest.Assets.Skills {
		name := fmt.Sprintf("assets.skills[%d]", i)
		if err := validateRelativePath(rel); err != nil {
			return fmt.Errorf("invalid %s: %w", name, err)
		}
		skillName := path.Base(rel)
		if strings.TrimSpace(skillName) == "" || rel != "skills/"+skillName {
			return fmt.Errorf("%s %q must match skills/<name>", name, rel)
		}
		if _, ok := seenSkills[skillName]; ok {
			return fmt.Errorf("%s has duplicate skill name %q", name, skillName)
		}
		seenSkills[skillName] = struct{}{}
	}
	seenConfigEnv := map[string]struct{}{}
	if !slices.IsSorted(manifest.Assets.ConfigEnv) {
		return fmt.Errorf("assets.configEnv must be sorted")
	}
	for i, name := range manifest.Assets.ConfigEnv {
		if err := validateEnvName(name); err != nil {
			return fmt.Errorf("invalid assets.configEnv[%d]: %w", i, err)
		}
		if _, ok := seenConfigEnv[name]; ok {
			return fmt.Errorf("assets.configEnv[%d] duplicates %q", i, name)
		}
		seenConfigEnv[name] = struct{}{}
	}
	for name := range manifest.Environment.Defaults {
		if err := validateEnvName(name); err != nil {
			return fmt.Errorf("invalid environment.defaults key %q: %w", name, err)
		}
	}
	for target, source := range manifest.Environment.Mappings {
		if err := validateEnvName(target); err != nil {
			return fmt.Errorf("invalid environment.mappings key %q: %w", target, err)
		}
		if err := validateEnvName(source); err != nil {
			return fmt.Errorf("invalid environment.mappings[%q]: %w", target, err)
		}
		if _, ok := manifest.Environment.Defaults[target]; ok {
			return fmt.Errorf("environment variable %q cannot have both a default and a mapping", target)
		}
	}
	return nil
}

func validateRelativePath(value string) error {
	if value == "" || value == "." || strings.Contains(value, "\\") || path.IsAbs(value) || isWindowsVolumePath(value) || path.Clean(value) != value || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("path %q must be a clean relative slash path", value)
	}
	return nil
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateEnvName(name string) error {
	if !envNamePattern.MatchString(name) {
		return fmt.Errorf("%q must match [A-Za-z_][A-Za-z0-9_]*", name)
	}
	if strings.HasPrefix(name, "AGENTFILE_") {
		return fmt.Errorf("%q must not start with reserved prefix AGENTFILE_", name)
	}
	return nil
}

func isWindowsVolumePath(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func validateUnpacked(unpacked *Unpacked) error {
	files := []string{ManifestName}
	for _, rel := range []string{
		unpacked.Manifest.Assets.Prompt,
		unpacked.Manifest.Assets.SystemPrompt,
		unpacked.Manifest.Assets.ConfigTemplate,
	} {
		if rel != "" {
			files = append(files, rel)
		}
	}
	for _, rel := range files {
		stat, err := os.Stat(filepath.Join(unpacked.Root, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("bundle asset %q is missing: %w", rel, err)
		}
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("bundle asset %q is not a regular file", rel)
		}
	}
	for _, rel := range unpacked.Manifest.Assets.Skills {
		stat, err := os.Stat(filepath.Join(unpacked.Root, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("bundle skill %q is missing: %w", rel, err)
		}
		if !stat.IsDir() {
			return fmt.Errorf("bundle skill %q is not a directory", rel)
		}
	}
	return nil
}
