package agentfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const DefaultFileName = "agentfile.yaml"

type Project struct {
	AgentFile     AgentFile
	ProjectDir    string
	AgentfilePath string
}

func Load(fileName string) (*Project, error) {
	agentfilePath, err := filepath.Abs(fileName)
	if err != nil {
		return nil, fmt.Errorf("resolve agentfile path: %w", err)
	}

	data, err := os.ReadFile(agentfilePath)
	if err != nil {
		return nil, fmt.Errorf("read agentfile: %w", err)
	}

	var af AgentFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&af); err != nil {
		return nil, fmt.Errorf("parse agentfile: %w", err)
	}
	if af.Metadata.Version == "" {
		af.Metadata.Version = DefaultVersion
	}

	project := &Project{
		AgentFile:     af,
		ProjectDir:    filepath.Dir(agentfilePath),
		AgentfilePath: agentfilePath,
	}
	if err := project.ApplyDiscovery(); err != nil {
		return nil, err
	}
	if err := project.AgentFile.Validate(); err != nil {
		return nil, err
	}
	return project, nil
}

func (p *Project) ApplyDiscovery() error {
	if p.AgentFile.Spec.Prompt == nil {
		promptPath := filepath.Join(p.ProjectDir, "prompt.md")
		if regularFileExists(promptPath) {
			src := Source{FS: &FilesystemSource{Path: "prompt.md"}}
			p.AgentFile.Spec.Prompt = &src
		}
	}

	if p.AgentFile.Spec.SystemPrompt == nil {
		systemPromptPath := filepath.Join(p.ProjectDir, "system-prompt.md")
		if regularFileExists(systemPromptPath) {
			src := Source{FS: &FilesystemSource{Path: "system-prompt.md"}}
			p.AgentFile.Spec.SystemPrompt = &src
		}
	}

	skillsDir := filepath.Join(p.ProjectDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read discovered skills: %w", err)
	}
	explicitSkills := map[string]struct{}{}
	for _, source := range p.AgentFile.Spec.Skills {
		if source.FS != nil && source.FS.Path != "" {
			explicitSkills[cleanRelativePath(source.FS.Path)] = struct{}{}
		}
	}
	var discovered []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if regularFileExists(skillPath) {
			sourcePath := filepath.ToSlash(filepath.Join("skills", entry.Name()))
			if _, ok := explicitSkills[cleanRelativePath(sourcePath)]; ok {
				continue
			}
			discovered = append(discovered, sourcePath)
		}
	}
	sort.Strings(discovered)
	for _, sourcePath := range discovered {
		p.AgentFile.Spec.Skills = append(p.AgentFile.Spec.Skills, Source{
			FS: &FilesystemSource{Path: sourcePath},
		})
	}
	return nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func cleanRelativePath(path string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

func (p *Project) DefaultImageTag() string {
	return p.AgentFile.Metadata.Name + ":" + p.AgentFile.Metadata.Version
}
