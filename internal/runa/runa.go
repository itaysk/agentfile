package runa

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
)

const Warning = "af: warning: bundle execution uses the current user without isolation or approval gates"

type Options struct {
	BundlePath    string
	Mode          harness.Mode
	Env           map[string]string
	EnvFiles      []string
	Workspace     string
	Prompt        *string
	Model         string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	WarningStderr io.Writer
	FailureStderr io.Writer
}

// Run extracts and invokes an agent bundle with a host-installed harness.
func Run(ctx context.Context, options Options) (int, error) {
	if options.BundlePath == "" {
		return 1, fmt.Errorf("bundle is required")
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
	if options.WarningStderr == nil {
		options.WarningStderr = options.Stderr
	}
	fmt.Fprintln(options.WarningStderr, Warning)

	tempDir, err := os.MkdirTemp("", "agentfile-runa-*")
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(tempDir)
	bundleRoot := filepath.Join(tempDir, "bundle")
	unpacked, err := bundle.Extract(options.BundlePath, bundleRoot)
	if err != nil {
		return 1, err
	}
	workspace, err := prepareWorkspace(tempDir, options.Workspace)
	if err != nil {
		return 1, err
	}
	env, err := InvocationEnv(options.EnvFiles, options.Env)
	if err != nil {
		return 1, err
	}
	command, err := harness.Prepare(unpacked, filepath.Join(tempDir, "profile"), harness.Invocation{
		Mode:      options.Mode,
		Workspace: workspace,
		Prompt:    options.Prompt,
		Model:     options.Model,
		Env:       env,
	})
	if err != nil {
		return 1, err
	}
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Dir = command.Dir
	cmd.Env = EnvList(command.Env)
	cmd.Stdin = options.Stdin
	cmd.Stdout = options.Stdout
	var failureStderr strings.Builder
	captureFailure := options.Mode == harness.ModeOneShot && options.FailureStderr != nil
	if captureFailure {
		cmd.Stderr = &failureStderr
	} else {
		cmd.Stderr = options.Stderr
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 1, fmt.Errorf("running a bundle requires %q on PATH: %w", command.Executable, err)
		}
		return 1, fmt.Errorf("start %s: %w", command.Executable, err)
	}
	stopSignals := forwardSignals(cmd)
	err = cmd.Wait()
	stopSignals()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if captureFailure && failureStderr.Len() > 0 {
				_, _ = io.WriteString(options.FailureStderr, failureStderr.String())
			}
			return exitError.ExitCode(), nil
		}
		return 1, fmt.Errorf("run %s: %w", command.Executable, err)
	}
	return 0, nil
}

func prepareWorkspace(tempDir, selected string) (string, error) {
	if selected == "" {
		workspace := filepath.Join(tempDir, "workspace")
		if err := os.Mkdir(workspace, 0o700); err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(workspace)
	}
	info, err := os.Stat(selected)
	if err != nil {
		return "", fmt.Errorf("read workspace host path %q: %w", selected, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace host path %q is not a directory", selected)
	}
	return filepath.EvalSymlinks(selected)
}

// InvocationEnv returns the inherited bundle environment with files and explicit values applied.
func InvocationEnv(files []string, explicit map[string]string) (map[string]string, error) {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	for _, path := range files {
		values, err := readEnvFile(path, env)
		if err != nil {
			return nil, err
		}
		for key, value := range values {
			env[key] = value
		}
	}
	for key, value := range explicit {
		env[key] = value
	}
	return env, nil
}

func readEnvFile(path string, env map[string]string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")
		key, value, hasValue := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("env file %q line %d has invalid variable name", path, line)
		}
		if !hasValue {
			value = env[key]
		} else {
			value = strings.TrimSpace(value)
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// EnvList converts an environment map to the form expected by exec.Cmd.
func EnvList(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		result = append(result, key+"="+env[key])
	}
	return result
}

func forwardSignals(cmd *exec.Cmd) func() {
	ch := make(chan os.Signal, 2)
	done := make(chan struct{})
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			select {
			case sig := <-ch:
				if cmd.Process == nil || cmd.Process.Signal(sig) != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
