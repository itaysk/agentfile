package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"

	"github.com/itaysk/agentfile/internal/agentfile"
	buildpkg "github.com/itaysk/agentfile/internal/build"
)

type Options struct {
	Project         *agentfile.Project
	Tag             string
	DockerBinary    string
	Env             map[string]string
	EnvFiles        []string
	Workspace       string
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	extraDockerArgs []string
}

func Run(ctx context.Context, options Options) (int, error) {
	if options.Project == nil {
		return 1, fmt.Errorf("project is required")
	}
	if options.Project.AgentFile.Spec.Prompt == nil {
		return 1, fmt.Errorf("run requires an effective prompt")
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
	forwardStdin := shouldForwardStdin(options.Stdin)

	tag, err := buildpkg.Build(ctx, buildpkg.Options{
		Project:      options.Project,
		Tag:          options.Tag,
		DockerBinary: options.DockerBinary,
		Stdout:       options.Stderr,
		Stderr:       options.Stderr,
	})
	if err != nil {
		return 1, err
	}

	args := []string{"run", "--rm"}
	if forwardStdin {
		args = append(args, "-i")
	}
	args = append(args, options.extraDockerArgs...)
	for _, envFile := range options.EnvFiles {
		args = append(args, "--env-file", envFile)
	}
	envs := runEnv(options.Project.AgentFile, options.Env)
	for _, key := range slices.Sorted(maps.Keys(envs)) {
		args = append(args, "-e", key+"="+envs[key])
	}
	if workspace != "" {
		args = append(args, "-v", workspace+":/agent/workspace")
	}
	args = append(args, tag)

	cmd := exec.CommandContext(ctx, options.DockerBinary, args...)
	if forwardStdin {
		cmd.Stdin = options.Stdin
	}
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode(), nil
		}
		return 1, fmt.Errorf("docker run failed: %w", err)
	}
	return 0, nil
}

func shouldForwardStdin(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return reader != nil
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

// runEnv merges explicit --env values with host-forwarded variables: exactly
// the runtimeEnv names declared in the spec, nothing implicit. Missing names
// are not an error here — an --env-file may supply them, and the entrypoint's
// guard is the authoritative failure point.
func runEnv(af agentfile.AgentFile, explicit map[string]string) map[string]string {
	envs := map[string]string{}
	for key, value := range explicit {
		envs[key] = value
	}
	for _, name := range af.Spec.RuntimeEnvNames() {
		if _, ok := envs[name]; ok {
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			envs[name] = value
		}
	}
	return envs
}
