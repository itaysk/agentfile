package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/itaysk/agentfile/internal/bundle"
)

const (
	MetadataLabel     = "build.agentfile.metadata"
	RuntimeEnvLabel   = "build.agentfile.runtimeEnv"
	HarnessLabel      = "build.agentfile.harness"
	BundleDigestLabel = "build.agentfile.bundle.digest"
)

type Options struct {
	BundlePath   string
	BaseImage    string
	Tag          string
	DockerBinary string
	Stdout       io.Writer
	Stderr       io.Writer
}

// Build creates an agent image from an agent bundle.
func Build(ctx context.Context, options Options) (string, error) {
	if options.BundlePath == "" {
		return "", fmt.Errorf("bundle is required")
	}
	if options.DockerBinary == "" {
		options.DockerBinary = "docker"
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	contextDir, err := os.MkdirTemp("", "agentfile-image-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(contextDir)
	unpacked, err := WriteBuildContext(contextDir, options.BundlePath, options.BaseImage)
	if err != nil {
		return "", err
	}
	tag := options.Tag
	if tag == "" {
		tag = unpacked.Manifest.Agent.Name + ":" + unpacked.Manifest.Agent.Version
	}
	metadata, _ := json.Marshal(unpacked.Manifest.Agent)
	runtimeEnvs, _ := json.Marshal(unpacked.Manifest.RuntimeEnvNames())
	args := []string{
		"build", "-t", tag,
		"--label", MetadataLabel + "=" + string(metadata),
		"--label", RuntimeEnvLabel + "=" + string(runtimeEnvs),
		"--label", HarnessLabel + "=" + unpacked.Manifest.Harness,
		"--label", BundleDigestLabel + "=" + unpacked.Digest,
	}
	for _, name := range slices.Sorted(maps.Keys(unpacked.Manifest.Environment.Defaults)) {
		args = append(args, "--build-arg", literalBuildArgName(name)+"="+unpacked.Manifest.Environment.Defaults[name])
	}
	args = append(args, contextDir)
	cmd := exec.CommandContext(ctx, options.DockerBinary, args...)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build failed: %w", err)
	}
	return tag, nil
}

// WriteBuildContext writes an image build context from an agent bundle.
func WriteBuildContext(contextDir, bundlePath, baseImage string) (*bundle.Unpacked, error) {
	bundleDir := filepath.Join(contextDir, "bundle")
	unpacked, err := bundle.Extract(bundlePath, bundleDir)
	if err != nil {
		return nil, err
	}
	if baseImage == "" {
		switch unpacked.Manifest.Harness {
		case "claudecode":
			baseImage = "itaysk/claudecode:latest"
		case "codex":
			baseImage = "itaysk/codex:latest"
		case "pi":
			baseImage = "itaysk/pi:latest"
		}
	}
	if baseImage == "" || strings.ContainsAny(baseImage, " \t\r\n") {
		return nil, fmt.Errorf("base image must be a non-empty image reference without whitespace")
	}
	entrypoint, err := EntrypointScript(unpacked.Manifest)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(contextDir, "entrypoint"), []byte(entrypoint), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(dockerfile(baseImage, unpacked.Manifest.Environment.Defaults)), 0o644); err != nil {
		return nil, err
	}
	return unpacked, nil
}

func dockerfile(base string, defaults map[string]string) string {
	var builder strings.Builder
	builder.WriteString("FROM " + base + "\n\n")
	for _, envName := range slices.Sorted(maps.Keys(defaults)) {
		name := literalBuildArgName(envName)
		builder.WriteString("ARG " + name + "\nENV " + envName + "=${" + name + "}\n")
	}
	if len(defaults) > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString(`COPY bundle /agent/bundle
COPY entrypoint /agent/entrypoint
RUN chmod +x /agent/entrypoint && mkdir -p /agent/workspace
WORKDIR /agent/workspace
ENTRYPOINT ["/agent/entrypoint"]
`)
	return builder.String()
}

func literalBuildArgName(envName string) string { return "AGENTFILE_LITERAL_" + envName }
