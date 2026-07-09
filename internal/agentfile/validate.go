package agentfile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RefTokenPrefix marks runtime-variable placeholders in generated harness
// config content between build and entrypoint render.
const RefTokenPrefix = "__AGENTFILE_REF_"

// reservedEnvPrefix is owned by the generated entrypoint (AGENTFILE_PROMPT,
// AGENTFILE_ESC_*, ...); user entries must stay out of it.
const reservedEnvPrefix = "AGENTFILE_"

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
	if af.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required after defaults are applied")
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
	if c := s.Harness.ClaudeCode; c != nil && c.Bare {
		if len(s.Skills) > 0 {
			return fmt.Errorf("spec.harness.claudecode.bare cannot be true with spec.skills: bare mode does not load skills")
		}
		for _, env := range s.Envs {
			if env.Name == "CLAUDE_CODE_OAUTH_TOKEN" {
				return fmt.Errorf("spec.harness.claudecode.bare cannot be true with env CLAUDE_CODE_OAUTH_TOKEN: bare mode does not read subscription tokens")
			}
		}
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
		for i := range m.HTTP.Headers {
			if err := m.HTTP.Headers[i].Validate(fmt.Sprintf("%s.http.headers[%d]", path, i)); err != nil {
				return err
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
	if strings.HasPrefix(e.Name, reservedEnvPrefix) {
		return fmt.Errorf("%s.name must not start with reserved prefix %s", path, reservedEnvPrefix)
	}
	return e.ValueSource.Validate(path)
}

func (h Header) Validate(path string) error {
	if h.Name == "" {
		return fmt.Errorf("%s.name is required", path)
	}
	if strings.HasPrefix(h.Name, reservedEnvPrefix) {
		return fmt.Errorf("%s.name must not start with reserved prefix %s", path, reservedEnvPrefix)
	}
	return h.ValueSource.Validate(path)
}

func (v ValueSource) Validate(path string) error {
	if v.TypeCount() != 1 {
		return fmt.Errorf("%s must set exactly one of value or runtimeEnv", path)
	}
	if v.Value != nil && strings.Contains(*v.Value, RefTokenPrefix) {
		return fmt.Errorf("%s.value must not contain %s", path, RefTokenPrefix)
	}
	if v.RuntimeEnv != nil {
		if !envNamePattern.MatchString(v.RuntimeEnv.Name) {
			return fmt.Errorf("%s.runtimeEnv.name must match [A-Za-z_][A-Za-z0-9_]*", path)
		}
		if strings.HasPrefix(v.RuntimeEnv.Name, reservedEnvPrefix) {
			return fmt.Errorf("%s.runtimeEnv.name must not start with reserved prefix %s", path, reservedEnvPrefix)
		}
	}
	return nil
}
