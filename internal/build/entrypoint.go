package build

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func entrypointScript(af agentfile.AgentFile, assets *agentfile.ResolvedAssets, configs []configFile) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -eu\n\n")

	// The no-colon expansions throughout (?, +) mean only *unset* variables
	// trigger guards; a variable set to "" is a value and is used verbatim.
	runtimeNames := af.Spec.RuntimeEnvNames()
	for _, name := range runtimeNames {
		builder.WriteString(`: "${` + name + `?agentfile: environment variable ` + name + ` is required}"` + "\n")
	}
	if len(runtimeNames) > 0 {
		builder.WriteString("\n")
	}

	writeRuntimeRender(&builder, af.Spec.ConfigRefNames(), configs)

	writeSpecEnvExports(&builder, af.Spec.Envs)
	if assets.HasPrompt {
		builder.WriteString(`if [ -z "${AGENTFILE_PROMPT+x}" ]; then AGENTFILE_PROMPT=`)
		builder.WriteString(shQuote(assets.Prompt))
		builder.WriteString("; fi\n")
	} else {
		builder.WriteString(`: "${AGENTFILE_PROMPT?agentfile: effective prompt is required}"` + "\n")
	}
	builder.WriteString("export AGENTFILE_PROMPT\n")
	builder.WriteString("AGENTFILE_PROVIDER=")
	builder.WriteString(shQuote(af.Spec.LLM.ProviderName()))
	builder.WriteString("\n")
	builder.WriteString(`if [ -z "${AGENTFILE_MODEL:-}" ]; then AGENTFILE_MODEL=`)
	builder.WriteString(shQuote(af.Spec.LLM.Model()))
	builder.WriteString("; fi\n")
	builder.WriteString("export AGENTFILE_PROVIDER AGENTFILE_MODEL\n")
	if assets.HasSystemPrompt {
		builder.WriteString("AGENTFILE_SYSTEM_PROMPT=")
		builder.WriteString(shQuote(assets.SystemPrompt))
		builder.WriteString("\n")
		builder.WriteString("export AGENTFILE_SYSTEM_PROMPT\n")
	}
	builder.WriteString("\ncd /agent/workspace\n\n")

	builder.WriteString("if [ -n \"${AGENTFILE_RENDER_ONLY:-}\" ]; then exit 0; fi\n\n")

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

// writeSpecEnvExports emits the spec.envs default-export lines: an entry
// already set in the container (`docker run -e NAME=…`) wins via the +x
// guard. Runtime entries copy from their source variable, which the required
// guard has already proven set.
func writeSpecEnvExports(builder *strings.Builder, envs []agentfile.Env) {
	for _, env := range envs {
		if env.RuntimeEnv != nil {
			builder.WriteString(`if [ -z "${` + env.Name + `+x}" ]; then export ` + env.Name + `="${` + env.RuntimeEnv.Name + `}"; fi` + "\n")
		} else {
			builder.WriteString(`if [ -z "${` + env.Name + `+x}" ]; then export ` + env.Name + `=` + shQuote(env.ValueString()) + `; fi` + "\n")
		}
	}
	if len(envs) > 0 {
		builder.WriteString("\n")
	}
}

// writeRuntimeRender emits the container-start substitution of runtime values
// into the harness config files, which are staged in the image with
// placeholder tokens: newline rejection and escaping for each referenced
// variable, then one in-place sed per config file that contains tokens. A
// file without runtime references is final as staged and never touched.
//
// AGENTFILE_ESC_<NAME> is escaped twice: once for the config format (JSON and
// TOML basic strings share the same two escape-critical characters, \ and "),
// then for use as a sed replacement (\, & and the , delimiter). Both escapes
// are line-based, hence the up-front newline rejection. The plain "$NAME"
// expansions are safe: the required guards already ran.
//
// The substitution writes through a temporary file and renames, so a
// container killed mid-render never leaves a half-substituted config; a
// re-run of the entrypoint in the same container is a no-op (no tokens left,
// and the container environment cannot have changed).
func writeRuntimeRender(builder *strings.Builder, refNames []string, configs []configFile) {
	for _, name := range refNames {
		builder.WriteString(`case "$` + name + `" in *"` + "\n" + `"*) echo "agentfile: ` + name + ` must not contain newlines" >&2; exit 64;; esac` + "\n")
		builder.WriteString(`AGENTFILE_ESC_` + name + `=$(printf '%s' "$` + name + `" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/[\\&,]/\\&/g')` + "\n")
	}
	if len(refNames) > 0 {
		builder.WriteString("\n")
	}
	for _, config := range configs {
		names := configRefNames(config.content)
		if len(names) == 0 {
			continue
		}
		commands := make([]string, 0, len(names))
		for _, name := range names {
			commands = append(commands, `s,`+refToken(name)+`,'"$AGENTFILE_ESC_`+name+`"',g`)
		}
		tmp := config.path + ".tmp"
		builder.WriteString(`sed '` + strings.Join(commands, "; ") + `' ` + shQuote(config.path) +
			` > ` + shQuote(tmp) + ` && mv ` + shQuote(tmp) + ` ` + shQuote(config.path) + "\n\n")
	}
}

// refTokenPattern matches placeholder tokens in generated config content.
// The greedy name match is unambiguous because generated JSON/TOML always
// quote-delimits tokens, so a token is never followed by a name char.
var refTokenPattern = regexp.MustCompile(regexp.QuoteMeta(agentfile.RefTokenPrefix) + `([A-Za-z_][A-Za-z0-9_]*)__`)

func configRefNames(content string) []string {
	set := map[string]struct{}{}
	for _, match := range refTokenPattern.FindAllStringSubmatch(content, -1) {
		set[match[1]] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

func claudeCodeEntrypoint(af agentfile.AgentFile, assets *agentfile.ResolvedAssets) string {
	args := []string{
		"claude",
		"--print",
		"--model \"$AGENTFILE_MODEL\"",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
	}
	if af.Spec.Harness.ClaudeCode != nil && af.Spec.Harness.ClaudeCode.Bare {
		args = append(args, "--bare")
	}
	if assets.HasSystemPrompt {
		args = append(args, "--system-prompt-file /agent/agentfile/system-prompt.md")
	}
	if len(af.Spec.MCPs) > 0 {
		args = append(args, "--mcp-config /agent/agentfile/claudecode/mcp.json", "--strict-mcp-config")
	}
	args = append(args, "\"$AGENTFILE_PROMPT\"")
	return `export HOME=/agent/agentfile/claudecode/home
export IS_SANDBOX=1
exec ` + strings.Join(args, " \\\n  ") + "\n"
}

func codexEntrypoint() string {
	return `export HOME=/agent/agentfile/codex/home
export CODEX_HOME=/agent/agentfile/codex/home/.codex
if [ -n "${CODEX_ACCESS_TOKEN:-}" ]; then
  unset CODEX_API_KEY
elif [ -n "${OPENAI_API_KEY:-}" ] && [ -z "${CODEX_API_KEY:-}" ]; then
  export CODEX_API_KEY="$OPENAI_API_KEY"
fi
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
	return `export PI_CODING_AGENT_DIR=/agent/agentfile/pi/home
exec ` + strings.Join(args, " \\\n  ") + "\n"
}

func shQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
