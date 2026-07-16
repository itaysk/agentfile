package image

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
)

// EntrypointScript returns the agent image entrypoint script.
// Agent images currently execute bundles here instead of through runa because
// base images do not provide a platform-specific runa executable. A future
// image format may package runa and use it as the entrypoint.
func EntrypointScript(manifest bundle.Manifest) (string, error) {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\nset -eu\n\n")
	builder.WriteString("AGENTFILE_RUN_MODE=${AGENTFILE_RUN_MODE:-oneshot}\n")
	builder.WriteString("case \"$AGENTFILE_RUN_MODE\" in\n  oneshot|tui|acp) ;;\n  *) echo \"agentfile: unsupported run mode $AGENTFILE_RUN_MODE\" >&2; exit 64;;\nesac\n\n")
	runtimeEnvNames := manifest.RuntimeEnvNames()
	for _, name := range runtimeEnvNames {
		builder.WriteString(`: "${` + name + `?agentfile: environment variable ` + name + ` is required}"` + "\n")
	}
	if len(runtimeEnvNames) > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString("AGENTFILE_UMASK=$(umask)\numask 077\nmkdir -p /agent/profile\nchmod 700 /agent/profile\n")
	appendProfileSetup(&builder, manifest)
	appendConfigRendering(&builder, manifest)
	builder.WriteString("umask \"$AGENTFILE_UMASK\"\nunset AGENTFILE_UMASK\n\n")
	appendDeclaredEnv(&builder, manifest.Environment)
	builder.WriteString("if [ \"$AGENTFILE_RUN_MODE\" = oneshot ]; then\n")
	if manifest.Assets.Prompt != "" {
		builder.WriteString(`  if [ -z "${AGENTFILE_PROMPT+x}" ]; then AGENTFILE_PROMPT=$(cat ` + shQuote("/agent/bundle/"+manifest.Assets.Prompt) + `; printf x); AGENTFILE_PROMPT=${AGENTFILE_PROMPT%x}; fi` + "\n")
	} else {
		builder.WriteString(`  : "${AGENTFILE_PROMPT?agentfile: effective prompt is required}"` + "\n")
	}
	builder.WriteString("  export AGENTFILE_PROMPT\nelse\n  unset AGENTFILE_PROMPT\nfi\n")
	builder.WriteString("AGENTFILE_PROVIDER=" + shQuote(manifest.Model.Provider) + "\n")
	builder.WriteString(`if [ -z "${AGENTFILE_MODEL:-}" ]; then AGENTFILE_MODEL=` + shQuote(manifest.Model.Name) + `; fi` + "\n")
	builder.WriteString("export AGENTFILE_PROVIDER AGENTFILE_MODEL\n")
	if manifest.Assets.SystemPrompt != "" && manifest.Harness == "pi" {
		builder.WriteString("AGENTFILE_SYSTEM_PROMPT=$(cat " + shQuote("/agent/bundle/"+manifest.Assets.SystemPrompt) + "; printf x)\nAGENTFILE_SYSTEM_PROMPT=${AGENTFILE_SYSTEM_PROMPT%x}\nexport AGENTFILE_SYSTEM_PROMPT\n")
	}
	builder.WriteString("\ncd /agent/workspace\n\n")
	if err := appendHarnessInvocation(&builder, manifest); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func appendProfileSetup(b *strings.Builder, manifest bundle.Manifest) {
	switch manifest.Harness {
	case "claudecode":
		b.WriteString("mkdir -p /agent/profile/claudecode/home/.claude/skills\n")
		for _, skill := range manifest.Assets.Skills {
			dest := "/agent/profile/claudecode/home/.claude/skills/" + path.Base(skill)
			b.WriteString("mkdir -p " + shQuote(dest) + "\n")
			b.WriteString("cp -R " + shQuote("/agent/bundle/"+skill+"/.") + " " + shQuote(dest) + "\n")
		}
	case "codex":
		b.WriteString("mkdir -p /agent/profile/codex/home/.codex /agent/profile/codex/home/.agents/skills\n")
		for _, skill := range manifest.Assets.Skills {
			dest := "/agent/profile/codex/home/.agents/skills/" + path.Base(skill)
			b.WriteString("mkdir -p " + shQuote(dest) + "\n")
			b.WriteString("cp -R " + shQuote("/agent/bundle/"+skill+"/.") + " " + shQuote(dest) + "\n")
		}
	case "pi":
		b.WriteString("mkdir -p /agent/profile/pi/home\n")
	}
	b.WriteString("\n")
}

func appendConfigRendering(b *strings.Builder, manifest bundle.Manifest) {
	configEnv := slices.Clone(manifest.Assets.ConfigEnv)
	slices.Sort(configEnv)
	for _, name := range configEnv {
		b.WriteString(`case "$` + name + `" in *"` + "\n" + `"*|*"$(printf '\r')"*) echo "agentfile: ` + name + ` must not contain newlines" >&2; exit 64;; esac` + "\n")
		b.WriteString(`AGENTFILE_ESC_` + name + `=$(printf '%s' "$` + name + `" | sed 's/\\/\\\\/g; s/"/\\"/g' | sed 's/[\\&,]/\\&/g')` + "\n")
	}
	if manifest.Assets.ConfigTemplate == "" {
		b.WriteString("\n")
		return
	}
	templatePath := "/agent/bundle/" + manifest.Assets.ConfigTemplate
	var configPath string
	switch manifest.Harness {
	case "claudecode":
		configPath = "/agent/profile/claudecode/mcp.json"
	case "codex":
		configPath = "/agent/profile/codex/home/.codex/config.toml"
	default:
		b.WriteString("\n")
		return
	}
	commands := []string{
		"s," + bundle.BundleRootToken + ",/agent/bundle,g",
		"s," + bundle.WorkspaceToken + ",/agent/workspace,g",
	}
	for _, name := range configEnv {
		commands = append(commands, "s,"+bundle.RefTokenPrefix+name+`__,'"$AGENTFILE_ESC_`+name+`"',g`)
	}
	b.WriteString("sed '" + strings.Join(commands, "; ") + "' " + shQuote(templatePath) + " > " + shQuote(configPath) + "\n")
	b.WriteString("chmod 600 " + shQuote(configPath) + "\n\n")
}

func appendDeclaredEnv(b *strings.Builder, environment bundle.Environment) {
	for _, name := range slices.Sorted(maps.Keys(environment.Defaults)) {
		b.WriteString(`if [ -z "${` + name + `+x}" ]; then export ` + name + `=` + shQuote(environment.Defaults[name]) + `; fi` + "\n")
	}
	for _, name := range slices.Sorted(maps.Keys(environment.Mappings)) {
		b.WriteString(`if [ -z "${` + name + `+x}" ]; then export ` + name + `="${` + environment.Mappings[name] + `}"; fi` + "\n")
	}
	if len(environment.Defaults)+len(environment.Mappings) > 0 {
		b.WriteString("\n")
	}
}

const (
	modelShellToken  = "__AGENTFILE_SHELL_MODEL__"
	promptShellToken = "__AGENTFILE_SHELL_PROMPT__"
	systemShellToken = "__AGENTFILE_SHELL_SYSTEM_PROMPT__"
)

func appendHarnessInvocation(b *strings.Builder, manifest bundle.Manifest) error {
	opts := harness.CommandOptions{
		BundleRoot:   "/agent/bundle",
		ProfileRoot:  "/agent/profile",
		Workspace:    "/agent/workspace",
		Model:        modelShellToken,
		Prompt:       promptShellToken,
		SystemPrompt: systemShellToken,
	}
	opts.Mode = harness.ModeTUI
	tui, err := harness.NewCommand(manifest, opts)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(tui.Env)) {
		b.WriteString("export " + key + "=" + shQuote(tui.Env[key]) + "\n")
	}
	if manifest.Harness == "claudecode" {
		b.WriteString("export IS_SANDBOX=1\n")
	}
	if manifest.Harness == "codex" {
		b.WriteString(`if [ -n "${CODEX_ACCESS_TOKEN:-}" ]; then
  unset CODEX_API_KEY
elif [ -n "${OPENAI_API_KEY:-}" ] && [ -z "${CODEX_API_KEY:-}" ]; then
  export CODEX_API_KEY="$OPENAI_API_KEY"
fi
if [ "$AGENTFILE_RUN_MODE" != oneshot ]; then
  if [ -n "${CODEX_ACCESS_TOKEN:-}" ]; then printf '%s' "$CODEX_ACCESS_TOKEN" | codex login --with-access-token >/dev/null 2>&1
  elif [ -n "${CODEX_API_KEY:-}" ]; then printf '%s' "$CODEX_API_KEY" | codex login --with-api-key >/dev/null 2>&1
  fi
fi
		`)
	}
	opts.Mode = harness.ModeACP
	acp, err := harness.NewCommand(manifest, opts)
	if err != nil {
		return err
	}
	opts.Mode = harness.ModeOneShot
	oneShot, err := harness.NewCommand(manifest, opts)
	if err != nil {
		return err
	}
	b.WriteString("if [ \"$AGENTFILE_RUN_MODE\" = tui ]; then exec " + shellCommand(tui) + "; fi\n")
	b.WriteString("if [ \"$AGENTFILE_RUN_MODE\" = acp ]; then exec " + shellCommand(acp) + "; fi\n")
	b.WriteString("exec " + shellCommand(oneShot) + "\n")
	return nil
}

func shellCommand(cmd *harness.Command) string {
	parts := []string{shQuote(cmd.Executable)}
	for _, arg := range cmd.Args {
		switch arg {
		case modelShellToken:
			parts = append(parts, `"$AGENTFILE_MODEL"`)
		case promptShellToken:
			parts = append(parts, `"$AGENTFILE_PROMPT"`)
		case systemShellToken:
			parts = append(parts, `"$AGENTFILE_SYSTEM_PROMPT"`)
		default:
			parts = append(parts, shQuote(arg))
		}
	}
	return strings.Join(parts, " ")
}

func shQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
