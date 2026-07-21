package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
)

const (
	Version           = "agentfile.build/bundle/v1"
	ManifestName      = "manifest.json"
	BundleRootToken   = "__AGENTFILE_BUNDLE_ROOT__"
	WorkspaceToken    = "__AGENTFILE_WORKSPACE__"
	RefTokenPrefix    = agentfile.RefTokenPrefix
	maxArchiveSize    = 1 << 30
	maxExtractedSize  = 512 << 20
	maxArchiveEntries = 100_000
)

// Manifest is the compiled bundle definition stored in manifest.json.
type Manifest struct {
	BundleVersion string      `json:"bundleVersion"`
	Agent         Agent       `json:"agent"`
	Harness       string      `json:"harness"`
	Bare          bool        `json:"bare,omitempty"`
	Model         Model       `json:"model"`
	Assets        Assets      `json:"assets"`
	Environment   Environment `json:"environment"`
}

type Agent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Model struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

type Assets struct {
	Prompt         string   `json:"prompt,omitempty"`
	SystemPrompt   string   `json:"systemPrompt,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	ConfigTemplate string   `json:"configTemplate,omitempty"`
	ConfigEnv      []string `json:"configEnv,omitempty"`
}

type Environment struct {
	Defaults map[string]string `json:"defaults,omitempty"`
	Mappings map[string]string `json:"mappings,omitempty"`
}

// Unpacked represents a validated bundle extracted at Root.
type Unpacked struct {
	Root     string
	Manifest Manifest
	Digest   string
}

// DefaultFilename returns a portable bundle filename derived from agent metadata.
func DefaultFilename(metadata agentfile.Metadata) string {
	clean := strings.NewReplacer("/", "-", "\\", "-").Replace
	return clean(metadata.Name) + "__" + clean(metadata.Version) + ".tar.gz"
}

// Build writes project to bundlePath as a reproducible agent bundle.
func Build(project *agentfile.Project, bundlePath string) error {
	if project == nil {
		return fmt.Errorf("project is required")
	}
	resolver, err := agentfile.NewResolver(project)
	if err != nil {
		return err
	}
	defer resolver.Close()
	assets, err := resolver.ResolveProject()
	if err != nil {
		return err
	}
	bundleRoot, err := os.MkdirTemp("", "agentfile-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(bundleRoot)
	if _, err := WriteLayout(bundleRoot, project, assets); err != nil {
		return err
	}
	return writeArchive(bundleRoot, bundlePath)
}

// WriteLayout writes resolved project assets to bundleRoot.
func WriteLayout(bundleRoot string, project *agentfile.Project, assets *agentfile.ResolvedAssets) (*Manifest, error) {
	if project == nil || assets == nil {
		return nil, fmt.Errorf("project and resolved assets are required")
	}
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		return nil, err
	}

	spec := project.AgentFile.Spec
	manifest := &Manifest{
		BundleVersion: Version,
		Agent: Agent{
			Name:    project.AgentFile.Metadata.Name,
			Version: project.AgentFile.Metadata.Version,
		},
		Harness: spec.Harness.Name(),
		Model: Model{
			Provider: spec.LLM.ProviderName(),
			Name:     spec.LLM.Model(),
		},
	}
	if spec.Harness.ClaudeCode != nil {
		manifest.Bare = spec.Harness.ClaudeCode.Bare
	}
	for _, env := range spec.Envs {
		if env.RuntimeEnv != nil {
			if manifest.Environment.Mappings == nil {
				manifest.Environment.Mappings = map[string]string{}
			}
			manifest.Environment.Mappings[env.Name] = env.RuntimeEnv.Name
			delete(manifest.Environment.Defaults, env.Name)
		} else {
			if manifest.Environment.Defaults == nil {
				manifest.Environment.Defaults = map[string]string{}
			}
			manifest.Environment.Defaults[env.Name] = env.LiteralValue()
			delete(manifest.Environment.Mappings, env.Name)
		}
	}
	if assets.HasPrompt {
		const rel = "prompt.md"
		manifest.Assets.Prompt = rel
		if err := os.WriteFile(filepath.Join(bundleRoot, rel), []byte(assets.Prompt), 0o644); err != nil {
			return nil, err
		}
	}
	if assets.HasSystemPrompt {
		const rel = "system-prompt.md"
		manifest.Assets.SystemPrompt = rel
		if err := os.WriteFile(filepath.Join(bundleRoot, rel), []byte(assets.SystemPrompt), 0o644); err != nil {
			return nil, err
		}
	}

	skills := append([]agentfile.ResolvedSkill(nil), assets.Skills...)
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	for _, skill := range skills {
		rel := filepath.ToSlash(filepath.Join("skills", skill.Name))
		manifest.Assets.Skills = append(manifest.Assets.Skills, rel)
		if err := copyTree(skill.Dir, filepath.Join(bundleRoot, filepath.FromSlash(rel))); err != nil {
			return nil, fmt.Errorf("copy skill %s: %w", skill.Name, err)
		}
	}

	if err := writeHarnessTemplates(bundleRoot, spec, manifest); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal bundle manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(bundleRoot, ManifestName), data, 0o644); err != nil {
		return nil, err
	}
	return manifest, nil
}

// RuntimeEnvNames returns the sorted environment variables required at run time.
func (m Manifest) RuntimeEnvNames() []string {
	set := map[string]struct{}{}
	for _, source := range m.Environment.Mappings {
		set[source] = struct{}{}
	}
	for _, name := range m.Assets.ConfigEnv {
		set[name] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if info.Mode()&0o111 != 0 {
				mode = 0o755
			}
			return os.WriteFile(target, data, mode)
		default:
			return fmt.Errorf("%s is not a regular file or directory", path)
		}
	})
}
