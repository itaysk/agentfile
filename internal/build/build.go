package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
	"gopkg.in/yaml.v3"
)

type Options struct {
	Project      *agentfile.Project
	Tag          string
	DockerBinary string
	Stdout       io.Writer
	Stderr       io.Writer
}

func Build(ctx context.Context, options Options) (string, error) {
	if options.Project == nil {
		return "", fmt.Errorf("project is required")
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
	tag := options.Tag
	if tag == "" {
		tag = options.Project.DefaultImageTag()
	}

	resolver, err := agentfile.NewResolver(options.Project.ProjectDir)
	if err != nil {
		return "", err
	}
	defer resolver.Close()

	assets, err := resolver.ResolveProject(options.Project)
	if err != nil {
		return "", err
	}

	contextDir, err := os.MkdirTemp("", "agentfile-build-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(contextDir)

	if err := StageContext(contextDir, options.Project, assets); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, options.DockerBinary, "build", "-t", tag, contextDir)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build failed: %w", err)
	}
	return tag, nil
}

func StageContext(contextDir string, project *agentfile.Project, assets *agentfile.ResolvedAssets) error {
	agentDir := filepath.Join(contextDir, "agentfile")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return err
	}

	effective, err := yaml.Marshal(project.AgentFile)
	if err != nil {
		return fmt.Errorf("marshal effective agentfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agentfile.effective.yaml"), effective, 0o644); err != nil {
		return err
	}
	if assets.HasPrompt {
		if err := os.WriteFile(filepath.Join(agentDir, "prompt.md"), []byte(assets.Prompt), 0o644); err != nil {
			return err
		}
	}
	if assets.HasSystemPrompt {
		if err := os.WriteFile(filepath.Join(agentDir, "system-prompt.md"), []byte(assets.SystemPrompt), 0o644); err != nil {
			return err
		}
	}

	if err := stageSkills(agentDir, project.AgentFile.Spec.Harness.Name(), assets.Skills); err != nil {
		return err
	}
	if err := stageHarnessHome(agentDir, project.AgentFile.Spec.Harness.Name()); err != nil {
		return err
	}
	configs, err := harnessConfigFiles(project.AgentFile, assets)
	if err != nil {
		return err
	}
	for _, config := range configs {
		dest := filepath.Join(contextDir, filepath.FromSlash(strings.TrimPrefix(config.path, "/agent/")))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(config.content), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(contextDir, "entrypoint"), []byte(entrypointScript(project.AgentFile, assets, configs)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(dockerfile(project.AgentFile.Spec.Harness.BaseImage())), 0o644)
}

var harnessHomes = map[string]string{
	"claudecode": "claudecode/home",
	"codex":      "codex/home/.codex",
	"pi":         "pi/home",
}

func stageHarnessHome(agentDir, harness string) error {
	home, ok := harnessHomes[harness]
	if !ok {
		return fmt.Errorf("unsupported harness %q", harness)
	}
	return os.MkdirAll(filepath.Join(agentDir, filepath.FromSlash(home)), 0o755)
}

func dockerfile(baseImage string) string {
	return fmt.Sprintf(`FROM %s

COPY agentfile /agent/agentfile
COPY entrypoint /agent/entrypoint
RUN chmod +x /agent/entrypoint && mkdir -p /agent/workspace
WORKDIR /agent/workspace
ENTRYPOINT ["/agent/entrypoint"]
`, baseImage)
}

func stageSkills(agentDir, harness string, skills []agentfile.ResolvedSkill) error {
	commonSkillDir := filepath.Join(agentDir, "skills")
	for _, skill := range skills {
		if err := CopyDir(skill.Dir, filepath.Join(commonSkillDir, skill.Name)); err != nil {
			return fmt.Errorf("copy skill %s: %w", skill.Name, err)
		}
	}

	var harnessSkillRoot string
	switch harness {
	case "claudecode":
		harnessSkillRoot = filepath.Join(agentDir, "claudecode", "home", ".claude", "skills")
	case "codex":
		harnessSkillRoot = filepath.Join(agentDir, "codex", "home", ".agents", "skills")
	default:
		return nil
	}
	for _, skill := range skills {
		if err := CopyDir(skill.Dir, filepath.Join(harnessSkillRoot, skill.Name)); err != nil {
			return fmt.Errorf("install skill %s for %s: %w", skill.Name, harness, err)
		}
	}
	return nil
}
