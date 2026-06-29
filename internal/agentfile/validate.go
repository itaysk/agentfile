package agentfile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (af AgentFile) Validate() error {
	if af.APIVersion != APIVersion {
		return fmt.Errorf("apiVersion must be %q", APIVersion)
	}
	if af.Kind != Kind {
		return fmt.Errorf("kind must be %q", Kind)
	}
	if strings.TrimSpace(af.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if af.Metadata.Version == nil {
		return fmt.Errorf("metadata.version is required after defaults are applied")
	}
	if *af.Metadata.Version == "" {
		return fmt.Errorf("metadata.version must be non-empty when present")
	}
	if err := af.Spec.Validate(); err != nil {
		return err
	}
	return nil
}

func (s Spec) Validate() error {
	if s.Harness.SelectorCount() != 1 {
		return fmt.Errorf("spec.harness must set exactly one of claudecode, codex, or pi")
	}
	if s.Harness.Image == "" && s.Harness.BaseImage() == "" {
		return fmt.Errorf("spec.harness has no supported selector")
	}
	if s.LLM.ProviderCount() != 1 {
		return fmt.Errorf("spec.llm must set exactly one of anthropic, openai, or openrouter")
	}
	if s.LLM.Model() == "" {
		return fmt.Errorf("spec.llm.%s.model is required", s.LLM.ProviderName())
	}
	if s.Prompt != nil {
		if err := s.Prompt.Validate("spec.prompt"); err != nil {
			return err
		}
	}
	if s.SystemPrompt != nil {
		if err := s.SystemPrompt.Validate("spec.systemPrompt"); err != nil {
			return err
		}
	}
	for i := range s.Skills {
		if err := s.Skills[i].Validate(fmt.Sprintf("spec.skills[%d]", i)); err != nil {
			return err
		}
	}
	seenMCPNames := map[string]struct{}{}
	for i := range s.MCPs {
		if err := s.MCPs[i].Validate(fmt.Sprintf("spec.mcps[%d]", i)); err != nil {
			return err
		}
		name := s.MCPs[i].Name
		if _, ok := seenMCPNames[name]; ok {
			return fmt.Errorf("spec.mcps[%d].name %q must be unique within spec.mcps", i, name)
		}
		seenMCPNames[name] = struct{}{}
	}
	for i := range s.Envs {
		if err := s.Envs[i].Validate(fmt.Sprintf("spec.envs[%d]", i)); err != nil {
			return err
		}
	}
	if s.Workspace.HostBindPath != "" && !filepath.IsAbs(s.Workspace.HostBindPath) {
		return fmt.Errorf("spec.workspace.hostBindPath must be absolute")
	}
	return validateHarnessProvider(s.Harness.Name(), s.LLM.ProviderName(), len(s.MCPs))
}

func validateHarnessProvider(harness, provider string, mcpCount int) error {
	switch harness {
	case "claudecode":
		if provider != "anthropic" {
			return fmt.Errorf("unsupported combination: claudecode harness supports anthropic llm only")
		}
	case "codex":
		if provider != "openai" {
			return fmt.Errorf("unsupported combination: codex harness supports openai llm only")
		}
	case "pi":
		if mcpCount > 0 {
			return fmt.Errorf("unsupported combination: pi harness does not support spec.mcps")
		}
	default:
		return fmt.Errorf("unsupported harness %q", harness)
	}
	return nil
}

func (s Source) Validate(path string) error {
	if s.TypeCount() != 1 {
		return fmt.Errorf("%s must set exactly one source type", path)
	}
	switch {
	case s.FS != nil:
		if (s.FS.Path == "") == (s.FS.AbsolutePath == "") {
			return fmt.Errorf("%s.fs must set exactly one of path or absolutePath", path)
		}
		if s.FS.Path != "" && filepath.IsAbs(s.FS.Path) {
			return fmt.Errorf("%s.fs.path must be relative", path)
		}
		if s.FS.AbsolutePath != "" && !filepath.IsAbs(s.FS.AbsolutePath) {
			return fmt.Errorf("%s.fs.absolutePath must be absolute", path)
		}
	case s.Git != nil:
		if s.Git.URL == "" {
			return fmt.Errorf("%s.git.url is required", path)
		}
		parsed, err := url.Parse(s.Git.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "ssh") {
			return fmt.Errorf("%s.git.url must use http, https, or ssh scheme", path)
		}
		if s.Git.Ref != "" && s.Git.Commit != "" {
			return fmt.Errorf("%s.git cannot set both ref and commit", path)
		}
	case s.HTTP != nil:
		if s.HTTP.URL == "" {
			return fmt.Errorf("%s.http.url is required", path)
		}
		parsed, err := url.ParseRequestURI(s.HTTP.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("%s.http.url must be a valid URL", path)
		}
	}
	return nil
}

func validatePathSegment(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("%s must be a single path segment", field)
	}
	return nil
}

func (m MCP) Validate(path string) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%s.name is required", path)
	}
	count := 0
	if m.Stdio != nil {
		count++
		if len(m.Stdio.Command) == 0 {
			return fmt.Errorf("%s.stdio.command is required", path)
		}
		for i, part := range m.Stdio.Command {
			if part == "" {
				return fmt.Errorf("%s.stdio.command[%d] must be non-empty", path, i)
			}
		}
		for i := range m.Stdio.Envs {
			if err := m.Stdio.Envs[i].Validate(fmt.Sprintf("%s.stdio.envs[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	if m.HTTP != nil {
		count++
		if m.HTTP.URL == "" {
			return fmt.Errorf("%s.http.url is required", path)
		}
		parsed, err := url.ParseRequestURI(m.HTTP.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("%s.http.url must be a valid URL", path)
		}
		for i, header := range m.HTTP.Headers {
			if header.Name == "" {
				return fmt.Errorf("%s.http.headers[%d].name is required", path, i)
			}
			if header.Value == nil {
				return fmt.Errorf("%s.http.headers[%d].value is required", path, i)
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("%s must set exactly one transport", path)
	}
	return nil
}

func (e Env) Validate(path string) error {
	if !envNamePattern.MatchString(e.Name) {
		return fmt.Errorf("%s.name must match [A-Za-z_][A-Za-z0-9_]*", path)
	}
	if e.Value == nil {
		return fmt.Errorf("%s.value is required", path)
	}
	return nil
}
