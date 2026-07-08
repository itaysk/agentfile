package agentfile

import "testing"

func TestApplyOverrideSupportsGenericScalarPaths(t *testing.T) {
	project := testProject()

	if err := project.ApplyOverride("harness.image", "example/agent:latest"); err != nil {
		t.Fatalf("ApplyOverride returned error: %v", err)
	}
	if got := project.AgentFile.Spec.Harness.Image; got != "example/agent:latest" {
		t.Fatalf("harness image = %q, want example/agent:latest", got)
	}
}

func TestApplyOverrideSupportsBoolFields(t *testing.T) {
	project := testProject()

	if err := project.ApplyOverride("harness.claudecode.bare", "true"); err != nil {
		t.Fatalf("ApplyOverride returned error: %v", err)
	}
	if !project.AgentFile.Spec.Harness.ClaudeCode.Bare {
		t.Fatalf("harness.claudecode.bare = false, want true")
	}
}

func TestApplyOverrideKeepsStringFieldsVerbatim(t *testing.T) {
	project := testProject()

	if err := project.ApplyOverride("prompt", "true"); err != nil {
		t.Fatalf("ApplyOverride returned error: %v", err)
	}
	if got := project.AgentFile.Spec.Prompt.Text; got == nil || *got != "true" {
		t.Fatalf("prompt text = %v, want the string \"true\"", got)
	}
}

func TestApplyOverrideRejectsListPaths(t *testing.T) {
	project := testProject()
	value := "info"
	project.AgentFile.Spec.Envs = []Env{{Name: "LOG_LEVEL", ValueSource: ValueSource{Value: &value}}}

	if err := project.ApplyOverride("envs.0.value", "debug"); err == nil {
		t.Fatalf("ApplyOverride returned nil, want list override error")
	}
	if got := project.AgentFile.Spec.Envs[0].ValueString(); got != "info" {
		t.Fatalf("env value = %q, want unchanged info", got)
	}
}

func TestApplyOverridePromptReplacesSourceWithText(t *testing.T) {
	project := testProject()
	project.AgentFile.Spec.Prompt = &Source{FS: &FilesystemSource{Path: "prompt.md"}}

	if err := project.ApplyOverride("prompt", "say hi"); err != nil {
		t.Fatalf("ApplyOverride returned error: %v", err)
	}
	if project.AgentFile.Spec.Prompt.Text == nil || *project.AgentFile.Spec.Prompt.Text != "say hi" {
		t.Fatalf("prompt = %#v, want text source", project.AgentFile.Spec.Prompt)
	}
	if project.AgentFile.Spec.Prompt.FS != nil {
		t.Fatalf("prompt fs source = %#v, want nil", project.AgentFile.Spec.Prompt.FS)
	}
}

func testProject() *Project {
	version := DefaultVersion
	return &Project{
		AgentFile: AgentFile{
			APIVersion: APIVersion,
			Kind:       Kind,
			Metadata: Metadata{
				Name:    "hello",
				Version: &version,
			},
			Spec: Spec{
				Harness: Harness{ClaudeCode: &ClaudeCodeHarness{}},
				LLM:     LLM{Anthropic: &ModelProvider{Model: "claude-haiku-4-5"}},
			},
		},
	}
}
