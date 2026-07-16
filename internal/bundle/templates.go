package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
)

func writeHarnessTemplates(bundleRoot string, spec agentfile.Spec, manifest *Manifest) error {
	var templatePath, content string
	var err error
	switch manifest.Harness {
	case "claudecode":
		if len(spec.MCPs) == 0 {
			return nil
		}
		templatePath = "harness/claudecode/mcp.json.tmpl"
		content, err = claudeTemplate(spec.MCPs)
	case "codex":
		templatePath = "harness/codex/config.toml.tmpl"
		content = codexTemplate(manifest.Assets.SystemPrompt, spec.MCPs)
	case "pi":
		return nil
	default:
		return fmt.Errorf("unsupported harness %q", manifest.Harness)
	}
	if err != nil {
		return err
	}
	manifest.Assets.ConfigTemplate = templatePath
	manifest.Assets.ConfigEnv = spec.ConfigEnvNames()
	target := filepath.Join(bundleRoot, filepath.FromSlash(templatePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func refToken(name string) string { return agentfile.RefTokenPrefix + name + "__" }

func configValue(value agentfile.ValueSource) string {
	if value.RuntimeEnv != nil {
		return refToken(value.RuntimeEnv.Name)
	}
	return value.LiteralValue()
}

func claudeTemplate(mcps []agentfile.MCP) (string, error) {
	servers := map[string]any{}
	for _, mcp := range mcps {
		if mcp.Stdio != nil {
			server := map[string]any{"type": "stdio", "command": mcp.Stdio.Command[0]}
			if len(mcp.Stdio.Command) > 1 {
				server["args"] = mcp.Stdio.Command[1:]
			}
			if len(mcp.Stdio.Envs) > 0 {
				envs := map[string]string{}
				for _, env := range mcp.Stdio.Envs {
					envs[env.Name] = configValue(env.ValueSource)
				}
				server["env"] = envs
			}
			servers[mcp.Name] = server
			continue
		}
		server := map[string]any{"type": "http", "url": mcp.HTTP.URL}
		if len(mcp.HTTP.Headers) > 0 {
			headers := map[string]string{}
			for _, header := range mcp.HTTP.Headers {
				headers[header.Name] = configValue(header.ValueSource)
			}
			server["headers"] = headers
		}
		servers[mcp.Name] = server
	}
	data, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	return string(data) + "\n", err
}

func codexTemplate(systemPrompt string, mcps []agentfile.MCP) string {
	var builder strings.Builder
	builder.WriteString("project_doc_max_bytes = 0\n")
	if systemPrompt != "" {
		builder.WriteString("model_instructions_file = ")
		builder.WriteString(tomlString(BundleRootToken + "/" + systemPrompt))
		builder.WriteString("\n")
	}
	builder.WriteString("\n[projects.")
	builder.WriteString(tomlString(WorkspaceToken))
	builder.WriteString("]\ntrust_level = \"trusted\"\n")
	if len(mcps) > 0 {
		builder.WriteString("\n")
		writeCodexMCPs(&builder, mcps)
	}
	return builder.String()
}

func writeCodexMCPs(builder *strings.Builder, mcps []agentfile.MCP) {
	for _, mcp := range mcps {
		table := "mcp_servers." + tomlString(mcp.Name)
		builder.WriteString("[" + table + "]\n")
		if mcp.Stdio != nil {
			builder.WriteString("command = " + tomlString(mcp.Stdio.Command[0]) + "\n")
			if len(mcp.Stdio.Command) > 1 {
				values := make([]string, len(mcp.Stdio.Command)-1)
				for i, value := range mcp.Stdio.Command[1:] {
					values[i] = tomlString(value)
				}
				builder.WriteString("args = [" + strings.Join(values, ", ") + "]\n")
			}
			if len(mcp.Stdio.Envs) > 0 {
				builder.WriteString("\n[" + table + ".env]\n")
				envs := append([]agentfile.Env(nil), mcp.Stdio.Envs...)
				sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
				for _, env := range envs {
					builder.WriteString(tomlString(env.Name) + " = " + tomlString(configValue(env.ValueSource)) + "\n")
				}
			}
		} else {
			builder.WriteString("url = " + tomlString(mcp.HTTP.URL) + "\n")
			if len(mcp.HTTP.Headers) > 0 {
				headers := map[string]string{}
				for _, header := range mcp.HTTP.Headers {
					headers[header.Name] = configValue(header.ValueSource)
				}
				builder.WriteString("http_headers = " + tomlMap(headers) + "\n")
			}
		}
		builder.WriteString("\n")
	}
}

func tomlString(value string) string { return strconv.Quote(value) }

func tomlMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buffer bytes.Buffer
	buffer.WriteString("{ ")
	for i, key := range keys {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(tomlString(key) + " = " + tomlString(values[key]))
	}
	buffer.WriteString(" }")
	return buffer.String()
}
