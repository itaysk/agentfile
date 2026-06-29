package build

import (
	"fmt"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func entrypointScript(af agentfile.AgentFile, assets *agentfile.ResolvedAssets) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -eu\n\n")
	for _, env := range af.Spec.Envs {
		builder.WriteString("if [ -z \"${")
		builder.WriteString(env.Name)
		builder.WriteString("+x}\" ]; then export ")
		builder.WriteString(env.Name)
		builder.WriteString("=")
		builder.WriteString(shQuote(env.ValueString()))
		builder.WriteString("; fi\n")
	}
	if len(af.Spec.Envs) > 0 {
		builder.WriteString("\n")
	}
	if assets.HasPrompt {
		builder.WriteString("AGENTFILE_PROMPT=")
		builder.WriteString(shQuote(assets.Prompt))
		builder.WriteString("\n")
		builder.WriteString("export AGENTFILE_PROMPT\n")
	} else {
		builder.WriteString("echo \"agentfile: effective prompt is required\" >&2\n")
		builder.WriteString("exit 64\n")
	}
	builder.WriteString("AGENTFILE_PROVIDER=")
	builder.WriteString(shQuote(af.Spec.LLM.ProviderName()))
	builder.WriteString("\n")
	builder.WriteString("AGENTFILE_MODEL=")
	builder.WriteString(shQuote(af.Spec.LLM.Model()))
	builder.WriteString("\n")
	builder.WriteString("export AGENTFILE_PROVIDER AGENTFILE_MODEL\n")
	if assets.HasSystemPrompt {
		builder.WriteString("AGENTFILE_SYSTEM_PROMPT=")
		builder.WriteString(shQuote(assets.SystemPrompt))
		builder.WriteString("\n")
		builder.WriteString("export AGENTFILE_SYSTEM_PROMPT\n")
	}
	builder.WriteString("\ncd /agent/workspace\n\n")

	switch af.Spec.Harness.Name() {
	case "claudecode":
		builder.WriteString(claudeCodeEntrypoint(af, assets))
	case "codex":
		builder.WriteString(codexEntrypoint())
	case "pi":
		builder.WriteString(piEntrypoint(assets))
	}
	return builder.String()
}

func claudeCodeEntrypoint(af agentfile.AgentFile, assets *agentfile.ResolvedAssets) string {
	args := []string{
		"claude",
		"--print",
		"--model \"$AGENTFILE_MODEL\"",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
	}
	if len(assets.Skills) == 0 {
		args = append(args, "--bare")
	}
	if assets.HasSystemPrompt {
		args = append(args, "--system-prompt-file /agent/agentfile/system-prompt.md")
	}
	if len(af.Spec.MCPs) > 0 {
		args = append(args, "--mcp-config /agent/agentfile/claudecode/mcp.json", "--strict-mcp-config")
	}
	args = append(args, "\"$AGENTFILE_PROMPT\"")
	return "export HOME=/agent/agentfile/claudecode/home\nexport IS_SANDBOX=1\nexec " + strings.Join(args, " \\\n  ") + "\n"
}

func codexEntrypoint() string {
	return `export HOME=/agent/agentfile/codex/home
export CODEX_HOME=/agent/agentfile/codex/home/.codex
if [ -n "${OPENAI_API_KEY:-}" ] && [ -z "${CODEX_API_KEY:-}" ]; then export CODEX_API_KEY="$OPENAI_API_KEY"; fi
exec codex exec \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  --model "$AGENTFILE_MODEL" \
  "$AGENTFILE_PROMPT"
`
}

func piEntrypoint(assets *agentfile.ResolvedAssets) string {
	args := []string{
		"pi",
		"-p",
		"--provider \"$AGENTFILE_PROVIDER\"",
		"--model \"$AGENTFILE_MODEL\"",
		"--no-context-files",
	}
	if assets.HasSystemPrompt {
		args = append(args, "--system-prompt \"$AGENTFILE_SYSTEM_PROMPT\"")
	}
	for _, skill := range assets.Skills {
		args = append(args, fmt.Sprintf("--skill %s", shQuote("/agent/agentfile/skills/"+skill.Name)))
	}
	args = append(args, "\"$AGENTFILE_PROMPT\"")
	return "export PI_CODING_AGENT_DIR=/agent/agentfile/pi/home\nexec " + strings.Join(args, " \\\n  ") + "\n"
}

func shQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
