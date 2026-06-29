package agentfile

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
	Name    string  `yaml:"name" json:"name"`
	Version *string `yaml:"version,omitempty" json:"version,omitempty"`
}

type Spec struct {
	Harness      Harness   `yaml:"harness" json:"harness"`
	LLM          LLM       `yaml:"llm" json:"llm"`
	Prompt       *Source   `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	SystemPrompt *Source   `yaml:"systemPrompt,omitempty" json:"systemPrompt,omitempty"`
	Skills       []Source  `yaml:"skills,omitempty" json:"skills,omitempty"`
	MCPs         []MCP     `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	Envs         []Env     `yaml:"envs,omitempty" json:"envs,omitempty"`
	Workspace    Workspace `yaml:"workspace,omitempty" json:"workspace,omitempty"`
}

type Harness struct {
	Image      string       `yaml:"image,omitempty" json:"image,omitempty"`
	ClaudeCode *EmptyObject `yaml:"claudecode,omitempty" json:"claudecode,omitempty"`
	Codex      *EmptyObject `yaml:"codex,omitempty" json:"codex,omitempty"`
	Pi         *EmptyObject `yaml:"pi,omitempty" json:"pi,omitempty"`
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

type Env struct {
	Name  string  `yaml:"name" json:"name"`
	Value *string `yaml:"value" json:"value"`
}

type Header struct {
	Name  string  `yaml:"name" json:"name"`
	Value *string `yaml:"value" json:"value"`
}

type Workspace struct {
	HostBindPath string `yaml:"hostBindPath,omitempty" json:"hostBindPath,omitempty"`
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

func ProviderCredentialEnv(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return ""
	}
}

func (e Env) ValueString() string {
	if e.Value == nil {
		return ""
	}
	return *e.Value
}

func (h Header) ValueString() string {
	if h.Value == nil {
		return ""
	}
	return *h.Value
}
