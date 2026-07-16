package harness

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/itaysk/agentfile/internal/bundle"
)

type Mode string

const (
	ModeOneShot Mode = "oneshot"
	ModeTUI     Mode = "tui"
	ModeACP     Mode = "acp"
)

// Invocation describes a harness invocation.
type Invocation struct {
	Mode      Mode
	Workspace string
	Prompt    *string
	Model     string
	Env       map[string]string
}

// Command describes a harness command.
type Command struct {
	Executable string
	Args       []string
	Env        map[string]string
	Dir        string
}

// CommandOptions contains the resolved values needed by NewCommand.
type CommandOptions struct {
	Mode         Mode
	BundleRoot   string
	ProfileRoot  string
	Workspace    string
	Model        string
	Prompt       string
	SystemPrompt string
	Env          map[string]string
}

// Prepare prepares a harness profile under profileRoot and returns the harness command.
func Prepare(unpacked *bundle.Unpacked, profileRoot string, inv Invocation) (*Command, error) {
	if unpacked == nil || unpacked.Root == "" {
		return nil, fmt.Errorf("unpacked bundle is required")
	}
	if profileRoot == "" {
		return nil, fmt.Errorf("profile root is required")
	}
	bundleRoot := unpacked.Root
	manifest := unpacked.Manifest
	if inv.Mode == "" {
		inv.Mode = ModeOneShot
	}
	if inv.Mode != ModeOneShot && inv.Mode != ModeTUI && inv.Mode != ModeACP {
		return nil, fmt.Errorf("unsupported execution mode %q", inv.Mode)
	}
	if inv.Workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	env := maps.Clone(inv.Env)
	if env == nil {
		env = map[string]string{}
	}
	for _, name := range manifest.RuntimeEnvNames() {
		if _, ok := env[name]; !ok {
			return nil, fmt.Errorf("environment variable %s is required", name)
		}
	}
	for name, value := range manifest.Environment.Defaults {
		if _, exists := env[name]; exists {
			continue
		}
		env[name] = value
	}
	for name, source := range manifest.Environment.Mappings {
		if _, exists := env[name]; exists {
			continue
		}
		env[name] = env[source]
	}

	opts := CommandOptions{
		Mode:        inv.Mode,
		BundleRoot:  bundleRoot,
		ProfileRoot: profileRoot,
		Workspace:   inv.Workspace,
		Model:       inv.Model,
		Env:         env,
	}
	if opts.Model == "" {
		opts.Model = manifest.Model.Name
	}
	if inv.Mode == ModeOneShot {
		prompt, err := resolvePrompt(bundleRoot, manifest, inv.Prompt)
		if err != nil {
			return nil, err
		}
		opts.Prompt = prompt
	}
	if manifest.Harness == "pi" && manifest.Assets.SystemPrompt != "" {
		data, err := os.ReadFile(filepath.Join(bundleRoot, filepath.FromSlash(manifest.Assets.SystemPrompt)))
		if err != nil {
			return nil, err
		}
		opts.SystemPrompt = string(data)
	}

	if err := os.MkdirAll(profileRoot, 0o700); err != nil {
		return nil, err
	}
	if err := prepareProfile(bundleRoot, profileRoot, inv.Workspace, manifest, env); err != nil {
		return nil, err
	}
	return NewCommand(manifest, opts)
}

func resolvePrompt(bundleRoot string, manifest bundle.Manifest, override *string) (string, error) {
	if override != nil {
		return *override, nil
	}
	if manifest.Assets.Prompt == "" {
		return "", fmt.Errorf("run requires an effective prompt")
	}
	data, err := os.ReadFile(filepath.Join(bundleRoot, filepath.FromSlash(manifest.Assets.Prompt)))
	if err != nil {
		return "", fmt.Errorf("read bundle prompt: %w", err)
	}
	return string(data), nil
}

func prepareProfile(bundleRoot, profileRoot, workspace string, manifest bundle.Manifest, env map[string]string) error {
	var skillRoot string
	switch manifest.Harness {
	case "claudecode":
		skillRoot = filepath.Join(profileRoot, "claudecode", "home", ".claude", "skills")
		if manifest.Assets.ConfigTemplate != "" {
			if err := renderConfig(bundleRoot, workspace, env,
				filepath.Join(bundleRoot, filepath.FromSlash(manifest.Assets.ConfigTemplate)),
				filepath.Join(profileRoot, "claudecode", "mcp.json")); err != nil {
				return err
			}
		}
	case "codex":
		skillRoot = filepath.Join(profileRoot, "codex", "home", ".agents", "skills")
		if manifest.Assets.ConfigTemplate != "" {
			if err := renderConfig(bundleRoot, workspace, env,
				filepath.Join(bundleRoot, filepath.FromSlash(manifest.Assets.ConfigTemplate)),
				filepath.Join(profileRoot, "codex", "home", ".codex", "config.toml")); err != nil {
				return err
			}
		}
	case "pi":
		return os.MkdirAll(filepath.Join(profileRoot, "pi", "home"), 0o700)
	}
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		return err
	}
	for _, skill := range manifest.Assets.Skills {
		src := filepath.Join(bundleRoot, filepath.FromSlash(skill))
		name := path.Base(skill)
		if err := copyTree(src, filepath.Join(skillRoot, name)); err != nil {
			return err
		}
	}
	return nil
}

func renderConfig(bundleRoot, workspace string, env map[string]string, templatePath, outputPath string) error {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(data), bundle.BundleRootToken, escapeConfig(bundleRoot))
	content = strings.ReplaceAll(content, bundle.WorkspaceToken, escapeConfig(workspace))
	for _, match := range refPattern.FindAllStringSubmatch(content, -1) {
		name := match[1]
		value, ok := env[name]
		if !ok {
			return fmt.Errorf("environment variable %s is required", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s must not contain newlines", name)
		}
		content = strings.ReplaceAll(content, match[0], escapeConfig(value))
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(content), 0o600)
}

var refPattern = regexp.MustCompile(regexp.QuoteMeta(bundle.RefTokenPrefix) + `([A-Za-z_][A-Za-z0-9_]*)__`)

func escapeConfig(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

// NewCommand returns the harness command described by manifest and opts.
func NewCommand(manifest bundle.Manifest, opts CommandOptions) (*Command, error) {
	if opts.Mode != ModeOneShot && opts.Mode != ModeTUI && opts.Mode != ModeACP {
		return nil, fmt.Errorf("unsupported execution mode %q", opts.Mode)
	}
	command := &Command{Env: maps.Clone(opts.Env), Dir: opts.Workspace}
	if command.Env == nil {
		command.Env = map[string]string{}
	}
	switch manifest.Harness {
	case "claudecode":
		command.Executable = "claude"
		command.Env["HOME"] = filepath.Join(opts.ProfileRoot, "claudecode", "home")
		command.Env["IS_DEMO"] = "1"
		args := []string{"--model", opts.Model, "--dangerously-skip-permissions"}
		if manifest.Bare {
			args = append(args, "--bare")
		}
		if manifest.Assets.SystemPrompt != "" {
			args = append(args, "--system-prompt-file", filepath.Join(opts.BundleRoot, filepath.FromSlash(manifest.Assets.SystemPrompt)))
		}
		if manifest.Assets.ConfigTemplate != "" {
			args = append(args, "--mcp-config", filepath.Join(opts.ProfileRoot, "claudecode", "mcp.json"), "--strict-mcp-config")
		}
		switch opts.Mode {
		case ModeOneShot:
			command.Args = append([]string{"--print"}, args...)
			command.Args = append(command.Args, opts.Prompt)
		case ModeACP:
			command.Args = append([]string{"--output-format", "stream-json", "--verbose"}, args...)
			command.Args = append(command.Args, "--input-format", "stream-json", "--include-partial-messages")
		default:
			command.Args = args
		}
	case "codex":
		command.Executable = "codex"
		home := filepath.Join(opts.ProfileRoot, "codex", "home")
		command.Env["HOME"] = home
		command.Env["CODEX_HOME"] = filepath.Join(home, ".codex")
		if command.Env["CODEX_ACCESS_TOKEN"] != "" {
			delete(command.Env, "CODEX_API_KEY")
		} else if command.Env["CODEX_API_KEY"] == "" && command.Env["OPENAI_API_KEY"] != "" {
			command.Env["CODEX_API_KEY"] = command.Env["OPENAI_API_KEY"]
		}
		args := []string{"--dangerously-bypass-approvals-and-sandbox", "--model", opts.Model}
		switch opts.Mode {
		case ModeOneShot:
			command.Args = append([]string{"exec", "--skip-git-repo-check"}, args...)
			command.Args = append(command.Args, opts.Prompt)
		case ModeACP:
			command.Args = append(args, "app-server")
		default:
			command.Args = args
		}
	case "pi":
		command.Executable = "pi"
		command.Env["PI_CODING_AGENT_DIR"] = filepath.Join(opts.ProfileRoot, "pi", "home")
		args := []string{"--provider", manifest.Model.Provider, "--model", opts.Model, "--no-context-files"}
		if manifest.Assets.SystemPrompt != "" {
			args = append(args, "--system-prompt", opts.SystemPrompt)
		}
		for _, skill := range manifest.Assets.Skills {
			args = append(args, "--skill", filepath.Join(opts.BundleRoot, filepath.FromSlash(skill)))
		}
		switch opts.Mode {
		case ModeOneShot:
			command.Args = append([]string{"-p"}, args...)
			command.Args = append(command.Args, opts.Prompt)
		case ModeACP:
			command.Args = append([]string{"--mode", "rpc"}, args...)
		default:
			command.Args = args
		}
	default:
		return nil, fmt.Errorf("unsupported harness %q", manifest.Harness)
	}
	return command, nil
}

func copyTree(src, dst string) error {
	return os.CopyFS(dst, os.DirFS(src))
}
