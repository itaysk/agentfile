package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
	imagepkg "github.com/itaysk/agentfile/internal/image"
	"github.com/itaysk/agentfile/internal/runa"
)

type Options struct {
	Project           *agentfile.Project
	Tag               string
	BaseImage         string
	DockerBinary      string
	Env               map[string]string
	EnvFiles          []string
	InheritRuntimeEnv bool
	Workspace         string
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
	FailureStderr     io.Writer
	WarningStderr     io.Writer
	ImageRef          string
	BundlePath        string
	HarnessName       string
	RuntimeEnvNames   []string
	Prompt            *string
	Model             string
	Mode              harness.Mode
	extraDockerArgs   []string
}

// Run invokes a project, agent bundle, or agent image.
func Run(ctx context.Context, options Options) (int, error) {
	inputs := 0
	if options.Project != nil {
		inputs++
	}
	if options.ImageRef != "" {
		inputs++
	}
	if options.BundlePath != "" {
		inputs++
	}
	if inputs != 1 {
		return 1, fmt.Errorf("exactly one of project, bundle, or image is required")
	}
	if options.Mode == "" {
		options.Mode = harness.ModeOneShot
	}
	if options.DockerBinary == "" {
		options.DockerBinary = "docker"
	}
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if options.Env == nil {
		options.Env = map[string]string{}
	}
	if options.BundlePath != "" && options.Mode != harness.ModeACP {
		return runBundle(ctx, options)
	}
	switch options.Mode {
	case harness.ModeOneShot, harness.ModeTUI:
		return runDocker(ctx, options)
	case harness.ModeACP:
		return runACP(ctx, options)
	default:
		return 1, fmt.Errorf("unsupported run mode %q", options.Mode)
	}
}

func runBundle(ctx context.Context, options Options) (int, error) {
	if options.Mode == harness.ModeTUI && options.Prompt != nil {
		return 1, fmt.Errorf("--prompt cannot be used with --tui")
	}
	return runa.Run(ctx, runa.Options{
		BundlePath:    options.BundlePath,
		Mode:          options.Mode,
		Env:           options.Env,
		EnvFiles:      options.EnvFiles,
		Workspace:     options.Workspace,
		Prompt:        options.Prompt,
		Model:         options.Model,
		Stdin:         options.Stdin,
		Stdout:        options.Stdout,
		Stderr:        options.Stderr,
		WarningStderr: options.WarningStderr,
		FailureStderr: options.FailureStderr,
	})
}

func runDocker(ctx context.Context, options Options) (int, error) {
	harnessName := options.HarnessName
	if options.Project != nil {
		harnessName = options.Project.AgentFile.Spec.Harness.Name()
	}
	if options.Mode == harness.ModeTUI {
		if options.Prompt != nil {
			return 1, fmt.Errorf("--prompt cannot be used with --tui")
		}
		if harnessName == "" {
			return 1, fmt.Errorf("image %q is missing required %s label", options.ImageRef, imagepkg.HarnessLabel)
		}
	}
	if options.Mode == harness.ModeOneShot && options.ImageRef == "" && options.Project.AgentFile.Spec.Prompt == nil && options.Prompt == nil {
		return 1, fmt.Errorf("run requires an effective prompt")
	}
	workspace := options.Workspace
	if workspace != "" {
		info, err := os.Stat(workspace)
		if err != nil {
			if os.IsNotExist(err) {
				return 1, fmt.Errorf("workspace host path %q does not exist", workspace)
			}
			return 1, fmt.Errorf("read workspace host path %q: %w", workspace, err)
		}
		if !info.IsDir() {
			return 1, fmt.Errorf("workspace host path %q is not a directory", workspace)
		}
	}
	forwardStdin := options.Mode == harness.ModeTUI || shouldForwardStdin(options.Stdin)

	imageRef, err := selectOrBuildImage(ctx, options)
	if err != nil {
		return 1, err
	}

	args := []string{"run", "--rm"}
	if options.Mode == harness.ModeTUI {
		args = append(args, "-it")
	} else if forwardStdin {
		args = append(args, "-i")
	}
	args = append(args, options.extraDockerArgs...)
	runtimeEnvNames := options.RuntimeEnvNames
	if len(runtimeEnvNames) == 0 && options.Project != nil {
		runtimeEnvNames = options.Project.AgentFile.Spec.RuntimeEnvNames()
	}
	env := dockerEnv(runtimeEnvNames, options.Env, options.InheritRuntimeEnv)
	if options.Mode == harness.ModeTUI {
		env["AGENTFILE_RUN_MODE"] = "tui"
	} else if options.Prompt != nil {
		env["AGENTFILE_PROMPT"] = *options.Prompt
	}
	if options.Model != "" {
		env["AGENTFILE_MODEL"] = options.Model
	}
	args = appendDockerEnv(args, options.EnvFiles, env)
	if workspace != "" {
		args = append(args, "--mount", "type=bind,source="+workspace+",target=/agent/workspace")
	}
	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, options.DockerBinary, args...)
	if forwardStdin {
		cmd.Stdin = options.Stdin
	}
	cmd.Stdout = options.Stdout
	var failureStderr strings.Builder
	captureFailureStderr := options.Mode == harness.ModeOneShot && options.FailureStderr != nil
	if captureFailureStderr {
		cmd.Stderr = &failureStderr
	} else {
		cmd.Stderr = options.Stderr
	}
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if captureFailureStderr && failureStderr.Len() > 0 {
				_, _ = io.WriteString(options.FailureStderr, failureStderr.String())
			}
			return exitError.ExitCode(), nil
		}
		return 1, fmt.Errorf("docker run failed: %w", err)
	}
	return 0, nil
}

func selectOrBuildImage(ctx context.Context, options Options) (string, error) {
	if options.ImageRef != "" {
		return options.ImageRef, nil
	}
	return BuildImage(ctx, BuildImageOptions{
		Project:      options.Project,
		Tag:          options.Tag,
		BaseImage:    options.BaseImage,
		DockerBinary: options.DockerBinary,
		Stdout:       options.Stderr,
		Stderr:       options.Stderr,
	})
}

type BuildImageOptions struct {
	Project      *agentfile.Project
	BundlePath   string
	Tag          string
	BaseImage    string
	DockerBinary string
	Stdout       io.Writer
	Stderr       io.Writer
}

// BuildImage builds an agent image from a project or agent bundle.
func BuildImage(ctx context.Context, options BuildImageOptions) (string, error) {
	if (options.Project == nil) == (options.BundlePath == "") {
		return "", fmt.Errorf("exactly one of project or bundle is required")
	}
	bundlePath := options.BundlePath
	if options.Project != nil {
		tempDir, err := os.MkdirTemp("", "agentfile-image-bundle-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tempDir)
		bundlePath = filepath.Join(tempDir, "agent.tar.gz")
		if err := bundle.Build(options.Project, bundlePath); err != nil {
			return "", err
		}
	}
	return imagepkg.Build(ctx, imagepkg.Options{
		BundlePath:   bundlePath,
		BaseImage:    options.BaseImage,
		Tag:          options.Tag,
		DockerBinary: options.DockerBinary,
		Stdout:       options.Stdout,
		Stderr:       options.Stderr,
	})
}

func appendDockerEnv(args, files []string, env map[string]string) []string {
	for _, file := range files {
		args = append(args, "--env-file", file)
	}
	for _, key := range slices.Sorted(maps.Keys(env)) {
		args = append(args, "-e", key+"="+env[key])
	}
	return args
}

type ImageInfo struct {
	Metadata        agentfile.Metadata
	RuntimeEnvNames []string
	HarnessName     string
	BundleDigest    string
}

// ReadImageInfo reads agentfile metadata from a local image.
func ReadImageInfo(ctx context.Context, dockerBinary, ref string) (*ImageInfo, error) {
	if dockerBinary == "" {
		dockerBinary = "docker"
	}
	labels, err := inspectImageLabels(ctx, dockerBinary, ref)
	if err != nil {
		return nil, err
	}
	metadataLabel := labels[imagepkg.MetadataLabel]
	if metadataLabel == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, imagepkg.MetadataLabel)
	}
	runtimeEnvLabel := labels[imagepkg.RuntimeEnvLabel]
	if runtimeEnvLabel == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, imagepkg.RuntimeEnvLabel)
	}
	var info ImageInfo
	if err := json.Unmarshal([]byte(metadataLabel), &info.Metadata); err != nil {
		return nil, fmt.Errorf("parse %s label from image %q: %w", imagepkg.MetadataLabel, ref, err)
	}
	if strings.TrimSpace(info.Metadata.Name) == "" {
		return nil, fmt.Errorf("image %q has invalid %s label: metadata.name is required", ref, imagepkg.MetadataLabel)
	}
	if info.Metadata.Version == "" {
		return nil, fmt.Errorf("image %q has invalid %s label: metadata.version is required", ref, imagepkg.MetadataLabel)
	}
	if err := json.Unmarshal([]byte(runtimeEnvLabel), &info.RuntimeEnvNames); err != nil {
		return nil, fmt.Errorf("parse %s label from image %q: %w", imagepkg.RuntimeEnvLabel, ref, err)
	}
	info.HarnessName = labels[imagepkg.HarnessLabel]
	if info.HarnessName == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, imagepkg.HarnessLabel)
	}
	info.BundleDigest = labels[imagepkg.BundleDigestLabel]
	if info.BundleDigest == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, imagepkg.BundleDigestLabel)
	}
	return &info, nil
}

// PullImage fetches ref and writes Docker progress to stderr.
func PullImage(ctx context.Context, dockerBinary, ref string, stderr io.Writer) error {
	if dockerBinary == "" {
		dockerBinary = "docker"
	}
	pull := exec.CommandContext(ctx, dockerBinary, "pull", ref)
	pull.Stdout = stderr
	pull.Stderr = stderr
	if err := pull.Run(); err != nil {
		return fmt.Errorf("docker pull %q failed: %w", ref, err)
	}
	return nil
}

func inspectImageLabels(ctx context.Context, dockerBinary, ref string) (map[string]string, error) {
	output, err := exec.CommandContext(ctx, dockerBinary, "image", "inspect", "--format", "{{json .Config.Labels}}", ref).Output()
	if err != nil {
		return nil, fmt.Errorf("docker image inspect %q failed: %w", ref, err)
	}
	var labels map[string]string
	if err := json.Unmarshal(output, &labels); err != nil {
		return nil, fmt.Errorf("parse docker image labels for %q: %w", ref, err)
	}
	if labels == nil {
		labels = map[string]string{}
	}
	return labels, nil
}

func shouldForwardStdin(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return r != nil
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

func dockerEnv(names []string, explicit map[string]string, inherit bool) map[string]string {
	env := map[string]string{}
	for key, value := range explicit {
		env[key] = value
	}
	if !inherit {
		return env
	}
	for _, name := range names {
		if _, ok := env[name]; ok {
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			env[name] = value
		}
	}
	return env
}
