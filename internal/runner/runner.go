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
	"slices"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
	buildpkg "github.com/itaysk/agentfile/internal/build"
)

type Options struct {
	Project         *agentfile.Project
	Tag             string
	DockerBinary    string
	Env             map[string]string
	EnvFiles        []string
	EnvAuto         bool
	Workspace       string
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	FailureStderr   io.Writer
	Image           string
	Harness         string
	RuntimeEnvNames []string
	Prompt          *string
	Model           string
	Mode            RunMode
	extraDockerArgs []string
}

type RunMode string

const (
	RunModeOneShot RunMode = "oneshot"
	RunModeTUI     RunMode = "tui"
	RunModeACP     RunMode = "acp"
)

func Run(ctx context.Context, options Options) (int, error) {
	if options.Project == nil && options.Image == "" {
		return 1, fmt.Errorf("project is required")
	}
	if options.Mode == "" {
		options.Mode = RunModeOneShot
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
	switch options.Mode {
	case RunModeOneShot, RunModeTUI:
		return runContainer(ctx, options)
	case RunModeACP:
		return runACP(ctx, options)
	default:
		return 1, fmt.Errorf("unsupported run mode %q", options.Mode)
	}
}

func runContainer(ctx context.Context, options Options) (int, error) {
	harness := options.Harness
	if options.Project != nil {
		harness = options.Project.AgentFile.Spec.Harness.Name()
	}
	if options.Mode == RunModeTUI {
		if options.Prompt != nil {
			return 1, fmt.Errorf("--prompt cannot be used with --tui")
		}
		if harness == "" {
			return 1, fmt.Errorf("image %q predates TUI support (missing %s label); rebuild it with a current af", options.Image, buildpkg.HarnessLabel)
		}
	}
	if options.Mode == RunModeOneShot && options.Image == "" && options.Project.AgentFile.Spec.Prompt == nil && options.Prompt == nil {
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
	forwardStdin := options.Mode == RunModeTUI || shouldForwardStdin(options.Stdin)

	tag, err := resolveRunImage(ctx, options)
	if err != nil {
		return 1, err
	}

	args := []string{"run", "--rm"}
	if options.Mode == RunModeTUI {
		args = append(args, "-it")
	} else if forwardStdin {
		args = append(args, "-i")
	}
	args = append(args, options.extraDockerArgs...)
	runtimeEnvNames := options.RuntimeEnvNames
	if len(runtimeEnvNames) == 0 && options.Project != nil {
		runtimeEnvNames = options.Project.AgentFile.Spec.RuntimeEnvNames()
	}
	envs := runEnv(runtimeEnvNames, options.Env, options.EnvAuto)
	if options.Mode == RunModeTUI {
		envs["AGENTFILE_RUN_MODE"] = "tui"
	} else if options.Prompt != nil {
		envs["AGENTFILE_PROMPT"] = *options.Prompt
	}
	if options.Model != "" {
		envs["AGENTFILE_MODEL"] = options.Model
	}
	args = appendRunEnvironment(args, options.EnvFiles, envs)
	if workspace != "" {
		args = append(args, "--mount", "type=bind,source="+workspace+",target=/agent/workspace")
	}
	args = append(args, tag)

	cmd := exec.CommandContext(ctx, options.DockerBinary, args...)
	if forwardStdin {
		cmd.Stdin = options.Stdin
	}
	cmd.Stdout = options.Stdout
	var failureStderr strings.Builder
	captureFailureStderr := options.Mode == RunModeOneShot && options.FailureStderr != nil
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

func resolveRunImage(ctx context.Context, options Options) (string, error) {
	if options.Image != "" {
		return options.Image, nil
	}
	return buildpkg.Build(ctx, buildpkg.Options{
		Project:      options.Project,
		Tag:          options.Tag,
		DockerBinary: options.DockerBinary,
		Stdout:       options.Stderr,
		Stderr:       options.Stderr,
	})
}

func appendRunEnvironment(args, envFiles []string, envs map[string]string) []string {
	for _, envFile := range envFiles {
		args = append(args, "--env-file", envFile)
	}
	for _, key := range slices.Sorted(maps.Keys(envs)) {
		args = append(args, "-e", key+"="+envs[key])
	}
	return args
}

type ImageInfo struct {
	Metadata        agentfile.Metadata
	RuntimeEnvNames []string
	Harness         string
}

// ReadImageInfo reads agentfile labels from a local image. It never pulls;
// fetching the image is the caller's decision (see PullImage).
func ReadImageInfo(ctx context.Context, dockerBinary, ref string) (*ImageInfo, error) {
	if dockerBinary == "" {
		dockerBinary = "docker"
	}
	labels, err := inspectImageLabels(ctx, dockerBinary, ref)
	if err != nil {
		return nil, err
	}
	metadataLabel := labels[buildpkg.MetadataLabel]
	if metadataLabel == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, buildpkg.MetadataLabel)
	}
	runtimeEnvLabel := labels[buildpkg.RuntimeEnvLabel]
	if runtimeEnvLabel == "" {
		return nil, fmt.Errorf("image %q was not built by agentfile (missing %s label)", ref, buildpkg.RuntimeEnvLabel)
	}
	var info ImageInfo
	if err := json.Unmarshal([]byte(metadataLabel), &info.Metadata); err != nil {
		return nil, fmt.Errorf("parse %s label from image %q: %w", buildpkg.MetadataLabel, ref, err)
	}
	if strings.TrimSpace(info.Metadata.Name) == "" {
		return nil, fmt.Errorf("image %q has invalid %s label: metadata.name is required", ref, buildpkg.MetadataLabel)
	}
	if info.Metadata.Version == "" {
		return nil, fmt.Errorf("image %q has invalid %s label: metadata.version is required", ref, buildpkg.MetadataLabel)
	}
	if err := json.Unmarshal([]byte(runtimeEnvLabel), &info.RuntimeEnvNames); err != nil {
		return nil, fmt.Errorf("parse %s label from image %q: %w", buildpkg.RuntimeEnvLabel, ref, err)
	}
	info.Harness = labels[buildpkg.HarnessLabel]
	return &info, nil
}

// PullImage fetches ref, streaming docker's progress to stderr.
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

func shouldForwardStdin(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return reader != nil
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

// runEnv copies explicit --env values and, when enabled, merges exactly the
// runtimeEnv names declared in the spec. Missing names are not an error here —
// an --env-file may supply them, and the entrypoint's guard is authoritative.
func runEnv(runtimeEnvNames []string, explicit map[string]string, envAuto bool) map[string]string {
	envs := map[string]string{}
	for key, value := range explicit {
		envs[key] = value
	}
	if !envAuto {
		return envs
	}
	for _, name := range runtimeEnvNames {
		if _, ok := envs[name]; ok {
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			envs[name] = value
		}
	}
	return envs
}
