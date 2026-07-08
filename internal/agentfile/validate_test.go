package agentfile

import (
	"strings"
	"testing"
)

func stringPtr(value string) *string {
	return &value
}

func literal(value string) ValueSource {
	return ValueSource{Value: &value}
}

func runtime(name string) ValueSource {
	return ValueSource{RuntimeEnv: &RuntimeEnvSource{Name: name}}
}

func TestEnvValidateValueRuntimeEnvOneOf(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     Env
		wantErr string
	}{
		{name: "literal ok", env: Env{Name: "FOO", ValueSource: literal("bar")}},
		{name: "runtime ok", env: Env{Name: "FOO", ValueSource: runtime("BAR")}},
		{name: "runtime rename ok", env: Env{Name: "FOO", ValueSource: runtime("FOO")}},
		{name: "both set", env: Env{Name: "FOO", ValueSource: ValueSource{Value: stringPtr("bar"), RuntimeEnv: &RuntimeEnvSource{Name: "BAR"}}}, wantErr: "exactly one of value or runtimeEnv"},
		{name: "neither set", env: Env{Name: "FOO"}, wantErr: "exactly one of value or runtimeEnv"},
		{name: "bad runtime name", env: Env{Name: "FOO", ValueSource: runtime("1BAD")}, wantErr: "runtimeEnv.name must match"},
		{name: "reserved name", env: Env{Name: "AGENTFILE_FOO", ValueSource: literal("x")}, wantErr: "reserved prefix"},
		{name: "reserved runtime source", env: Env{Name: "FOO", ValueSource: runtime("AGENTFILE_BAR")}, wantErr: "reserved prefix"},
		{name: "literal contains ref token", env: Env{Name: "FOO", ValueSource: literal("x__AGENTFILE_REF_Y__z")}, wantErr: "must not contain __AGENTFILE_REF_"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.Validate("spec.envs[0]")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHeaderValidateValueRuntimeEnvOneOf(t *testing.T) {
	for _, tt := range []struct {
		name    string
		header  Header
		wantErr string
	}{
		{name: "literal ok", header: Header{Name: "Authorization", ValueSource: literal("Bearer x")}},
		{name: "runtime ok", header: Header{Name: "Authorization", ValueSource: runtime("SEARCH_MCP_AUTH")}},
		{name: "both set", header: Header{Name: "Authorization", ValueSource: ValueSource{Value: stringPtr("x"), RuntimeEnv: &RuntimeEnvSource{Name: "Y"}}}, wantErr: "exactly one of value or runtimeEnv"},
		{name: "neither set", header: Header{Name: "Authorization"}, wantErr: "exactly one of value or runtimeEnv"},
		{name: "bad runtime name", header: Header{Name: "Authorization", ValueSource: runtime("not-a-name")}, wantErr: "runtimeEnv.name must match"},
		{name: "reserved runtime source", header: Header{Name: "Authorization", ValueSource: runtime("AGENTFILE_X")}, wantErr: "reserved prefix"},
		{name: "missing name", header: Header{ValueSource: runtime("X")}, wantErr: "name is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.header.Validate("spec.mcps[0].http.headers[0]")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsBareTrueWithSkills(t *testing.T) {
	spec := Spec{
		Harness: Harness{ClaudeCode: &ClaudeCodeHarness{Bare: true}},
		LLM:     LLM{Anthropic: &ModelProvider{Model: "claude-haiku-4-5"}},
		Skills:  []Source{{FS: &FilesystemSource{Path: "skills/greet"}}},
	}
	err := spec.Validate()
	if err == nil || !strings.Contains(err.Error(), "bare cannot be true with spec.skills") {
		t.Fatalf("Validate = %v, want bare/skills conflict error", err)
	}
	spec.Skills = nil
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate without skills = %v, want nil", err)
	}

	spec.Envs = []Env{{Name: "CLAUDE_CODE_OAUTH_TOKEN", ValueSource: runtime("CLAUDE_CODE_OAUTH_TOKEN")}}
	err = spec.Validate()
	if err == nil || !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Fatalf("Validate = %v, want bare/subscription-token conflict error", err)
	}
}

func TestSpecRuntimeEnvNames(t *testing.T) {
	spec := Spec{
		Envs: []Env{
			{Name: "LOG_LEVEL", ValueSource: literal("info")},
			{Name: "GH_TOKEN", ValueSource: runtime("GITHUB_TOKEN")},
		},
		MCPs: []MCP{
			{
				Name: "github",
				Stdio: &StdioMCP{
					Command: []string{"github-mcp-server"},
					Envs:    []Env{{Name: "GITHUB_PERSONAL_ACCESS_TOKEN", ValueSource: runtime("GITHUB_TOKEN")}},
				},
			},
			{
				Name: "search",
				HTTP: &HTTPMCP{
					URL:     "https://example.com/mcp",
					Headers: []Header{{Name: "Authorization", ValueSource: runtime("SEARCH_MCP_AUTH")}},
				},
			},
		},
	}
	if got := strings.Join(spec.RuntimeEnvNames(), ","); got != "GITHUB_TOKEN,SEARCH_MCP_AUTH" {
		t.Fatalf("RuntimeEnvNames = %q, want all distinct names", got)
	}
	if got := strings.Join(spec.ConfigRefNames(), ","); got != "GITHUB_TOKEN,SEARCH_MCP_AUTH" {
		t.Fatalf("ConfigRefNames = %q, want config-referenced names", got)
	}
	specEnvsOnly := Spec{Envs: []Env{{Name: "FOO", ValueSource: runtime("BAR")}}}
	if len(specEnvsOnly.ConfigRefNames()) != 0 {
		t.Fatalf("ConfigRefNames = %v, want empty for spec.envs-only sources", specEnvsOnly.ConfigRefNames())
	}
	if got := strings.Join(specEnvsOnly.RuntimeEnvNames(), ","); got != "BAR" {
		t.Fatalf("RuntimeEnvNames = %q, want BAR", got)
	}
}
