package agentfile

import (
	"maps"
	"slices"
)

const (
	APIVersion = "agentfile.build/v1"
	Kind       = "Agent"

	DefaultVersion = "latest"
)

type AgentFile struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

type Spec struct {
	Harness      Harness  `yaml:"harness" json:"harness"`
	LLM          LLM      `yaml:"llm" json:"llm"`
	Prompt       *Source  `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	SystemPrompt *Source  `yaml:"systemPrompt,omitempty" json:"systemPrompt,omitempty"`
	Skills       []Source `yaml:"skills,omitempty" json:"skills,omitempty"`
	MCPs         []MCP    `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	Envs         []Env    `yaml:"envs,omitempty" json:"envs,omitempty"`
}

type Harness struct {
	Image      string             `yaml:"image,omitempty" json:"image,omitempty"`
	ClaudeCode *ClaudeCodeHarness `yaml:"claudecode,omitempty" json:"claudecode,omitempty"`
	Codex      *EmptyObject       `yaml:"codex,omitempty" json:"codex,omitempty"`
	Pi         *EmptyObject       `yaml:"pi,omitempty" json:"pi,omitempty"`
}

// ClaudeCodeHarness configures the Claude Code harness. Bare opts into
// claude's --bare mode (off by default). It is invalid alongside skills,
// which bare mode does not load, and incompatible with subscription auth,
// because bare mode does not read CLAUDE_CODE_OAUTH_TOKEN.
type ClaudeCodeHarness struct {
	Bare bool `yaml:"bare,omitempty" json:"bare,omitempty"`
}

type EmptyObject struct{}

type LLM struct {
	Anthropic  *ModelProvider `yaml:"anthropic,omitempty" json:"anthropic,omitempty"`
	OpenAI     *ModelProvider `yaml:"openai,omitempty" json:"openai,omitempty"`
	OpenRouter *ModelProvider `yaml:"openrouter,omitempty" json:"openrouter,omitempty"`
}

type ModelProvider struct {
	Model string `yaml:"model" json:"model"`
}

type Source struct {
	Text *string           `yaml:"text,omitempty" json:"text,omitempty"`
	FS   *FilesystemSource `yaml:"fs,omitempty" json:"fs,omitempty"`
	Git  *GitSource        `yaml:"git,omitempty" json:"git,omitempty"`
	HTTP *HTTPSource       `yaml:"http,omitempty" json:"http,omitempty"`
}

type FilesystemSource struct {
	Path         string `yaml:"path,omitempty" json:"path,omitempty"`
	AbsolutePath string `yaml:"absolutePath,omitempty" json:"absolutePath,omitempty"`
}

type GitSource struct {
	URL    string `yaml:"url" json:"url"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"`
}

type HTTPSource struct {
	URL     string `yaml:"url" json:"url"`
	Archive bool   `yaml:"archive,omitempty" json:"archive,omitempty"`
}

type MCP struct {
	Name  string    `yaml:"name" json:"name"`
	Stdio *StdioMCP `yaml:"stdio,omitempty" json:"stdio,omitempty"`
	HTTP  *HTTPMCP  `yaml:"http,omitempty" json:"http,omitempty"`
}

type StdioMCP struct {
	Command []string `yaml:"command" json:"command"`
	Envs    []Env    `yaml:"envs,omitempty" json:"envs,omitempty"`
}

type HTTPMCP struct {
	URL     string   `yaml:"url" json:"url"`
	Headers []Header `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// ValueSource is the one-of value of a name/value entry, mirroring the Source
// paradigm: exactly one member is set. New value sources are added here and
// take effect for envs and headers alike.
type ValueSource struct {
	Value      *string           `yaml:"value,omitempty" json:"value,omitempty"`
	RuntimeEnv *RuntimeEnvSource `yaml:"runtimeEnv,omitempty" json:"runtimeEnv,omitempty"`
}

// RuntimeEnvSource reads the entry's value from a container environment
// variable at container start. Referenced variables are required: the
// container fails when one is unset. Empty is a value: a variable set to ""
// is used verbatim and does not trigger the required guard. Runtime values
// never appear in image layers.
type RuntimeEnvSource struct {
	Name string `yaml:"name" json:"name"`
}

type Env struct {
	Name        string `yaml:"name" json:"name"`
	ValueSource `yaml:",inline"`
}

type Header struct {
	Name        string `yaml:"name" json:"name"`
	ValueSource `yaml:",inline"`
}

func (h Harness) Name() string {
	switch {
	case h.ClaudeCode != nil:
		return "claudecode"
	case h.Codex != nil:
		return "codex"
	case h.Pi != nil:
		return "pi"
	default:
		return ""
	}
}

func (h Harness) BaseImage() string {
	if h.Image != "" {
		return h.Image
	}
	switch h.Name() {
	case "claudecode":
		return "itaysk/claudecode:latest"
	case "codex":
		return "itaysk/codex:latest"
	case "pi":
		return "itaysk/pi:latest"
	default:
		return ""
	}
}

func (h Harness) SelectorCount() int {
	count := 0
	if h.ClaudeCode != nil {
		count++
	}
	if h.Codex != nil {
		count++
	}
	if h.Pi != nil {
		count++
	}
	return count
}

func (l LLM) ProviderName() string {
	switch {
	case l.Anthropic != nil:
		return "anthropic"
	case l.OpenAI != nil:
		return "openai"
	case l.OpenRouter != nil:
		return "openrouter"
	default:
		return ""
	}
}

func (l LLM) Model() string {
	switch {
	case l.Anthropic != nil:
		return l.Anthropic.Model
	case l.OpenAI != nil:
		return l.OpenAI.Model
	case l.OpenRouter != nil:
		return l.OpenRouter.Model
	default:
		return ""
	}
}

func (l LLM) ProviderCount() int {
	count := 0
	if l.Anthropic != nil {
		count++
	}
	if l.OpenAI != nil {
		count++
	}
	if l.OpenRouter != nil {
		count++
	}
	return count
}

func (s Source) TypeCount() int {
	count := 0
	if s.Text != nil {
		count++
	}
	if s.FS != nil {
		count++
	}
	if s.Git != nil {
		count++
	}
	if s.HTTP != nil {
		count++
	}
	return count
}

func TextSource(value string) Source {
	return Source{Text: &value}
}

func (v ValueSource) ValueString() string {
	if v.Value == nil {
		return ""
	}
	return *v.Value
}

func (v ValueSource) TypeCount() int {
	count := 0
	if v.Value != nil {
		count++
	}
	if v.RuntimeEnv != nil {
		count++
	}
	return count
}

// RuntimeEnvNames returns every distinct runtimeEnv name in the spec, sorted:
// the set the runner forwards from the host and the entrypoint requires at
// container start.
func (s Spec) RuntimeEnvNames() []string {
	set := map[string]struct{}{}
	for _, env := range s.Envs {
		if env.RuntimeEnv != nil {
			set[env.RuntimeEnv.Name] = struct{}{}
		}
	}
	for _, name := range s.ConfigRefNames() {
		set[name] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

// ConfigRefNames returns the distinct runtimeEnv names referenced by harness
// config files (MCP stdio envs and HTTP headers), sorted — these need
// JSON/TOML escaping at container start; spec.envs references do not.
func (s Spec) ConfigRefNames() []string {
	set := map[string]struct{}{}
	for _, mcp := range s.MCPs {
		if mcp.Stdio != nil {
			for _, env := range mcp.Stdio.Envs {
				if env.RuntimeEnv != nil {
					set[env.RuntimeEnv.Name] = struct{}{}
				}
			}
		}
		if mcp.HTTP != nil {
			for _, header := range mcp.HTTP.Headers {
				if header.RuntimeEnv != nil {
					set[header.RuntimeEnv.Name] = struct{}{}
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(set))
}
