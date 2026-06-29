package build

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

func writeHarnessConfig(agentDir string, af agentfile.AgentFile, assets *agentfile.ResolvedAssets) error {
	switch af.Spec.Harness.Name() {
	case "claudecode":
		return writeClaudeCodeConfig(agentDir, af)
	case "codex":
		return writeCodexConfig(agentDir, af, assets)
	case "pi":
		return os.MkdirAll(filepath.Join(agentDir, "pi", "home"), 0o755)
	default:
		return fmt.Errorf("unsupported harness %q", af.Spec.Harness.Name())
	}
}

func writeClaudeCodeConfig(agentDir string, af agentfile.AgentFile) error {
	if err := os.MkdirAll(filepath.Join(agentDir, "claudecode", "home"), 0o755); err != nil {
		return err
	}
	if len(af.Spec.MCPs) == 0 {
		return nil
	}
	config := map[string]any{
		"mcpServers": claudeMCPServers(af.Spec.MCPs),
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	configPath := filepath.Join(agentDir, "claudecode", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

func claudeMCPServers(mcps []agentfile.MCP) map[string]any {
	servers := map[string]any{}
	for _, mcp := range mcps {
		if mcp.Stdio != nil {
			server := map[string]any{
				"type":    "stdio",
				"command": mcp.Stdio.Command[0],
			}
			if len(mcp.Stdio.Command) > 1 {
				server["args"] = mcp.Stdio.Command[1:]
			}
			if len(mcp.Stdio.Envs) > 0 {
				envs := map[string]string{}
				for _, env := range mcp.Stdio.Envs {
					envs[env.Name] = env.ValueString()
				}
				server["env"] = envs
			}
			servers[mcp.Name] = server
			continue
		}
		server := map[string]any{
			"type": "http",
			"url":  mcp.HTTP.URL,
		}
		if len(mcp.HTTP.Headers) > 0 {
			headers := map[string]string{}
			for _, header := range mcp.HTTP.Headers {
				headers[header.Name] = header.ValueString()
			}
			server["headers"] = headers
		}
		servers[mcp.Name] = server
	}
	return servers
}

func writeCodexConfig(agentDir string, af agentfile.AgentFile, assets *agentfile.ResolvedAssets) error {
	configDir := filepath.Join(agentDir, "codex", "home", ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("project_doc_max_bytes = 0\n")
	if assets.HasSystemPrompt {
		builder.WriteString("model_instructions_file = ")
		builder.WriteString(tomlString("/agent/agentfile/system-prompt.md"))
		builder.WriteString("\n")
	}
	if len(af.Spec.MCPs) > 0 {
		builder.WriteString("\n")
		writeCodexMCPConfig(&builder, af.Spec.MCPs)
	}
	return os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(builder.String()), 0o644)
}

func writeCodexMCPConfig(builder *strings.Builder, mcps []agentfile.MCP) {
	for _, mcp := range mcps {
		table := "mcp_servers." + tomlString(mcp.Name)
		builder.WriteString("[")
		builder.WriteString(table)
		builder.WriteString("]\n")
		if mcp.Stdio != nil {
			builder.WriteString("command = ")
			builder.WriteString(tomlString(mcp.Stdio.Command[0]))
			builder.WriteString("\n")
			if len(mcp.Stdio.Command) > 1 {
				builder.WriteString("args = ")
				builder.WriteString(tomlStringArray(mcp.Stdio.Command[1:]))
				builder.WriteString("\n")
			}
			if len(mcp.Stdio.Envs) > 0 {
				builder.WriteString("\n[")
				builder.WriteString(table)
				builder.WriteString(".env]\n")
				envs := make([]agentfile.Env, len(mcp.Stdio.Envs))
				copy(envs, mcp.Stdio.Envs)
				sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
				for _, env := range envs {
					builder.WriteString(tomlString(env.Name))
					builder.WriteString(" = ")
					builder.WriteString(tomlString(env.ValueString()))
					builder.WriteString("\n")
				}
			}
		} else {
			builder.WriteString("url = ")
			builder.WriteString(tomlString(mcp.HTTP.URL))
			builder.WriteString("\n")
			if len(mcp.HTTP.Headers) > 0 {
				builder.WriteString("http_headers = ")
				builder.WriteString(tomlInlineStringMap(headersMap(mcp.HTTP.Headers)))
				builder.WriteString("\n")
			}
		}
		builder.WriteString("\n")
	}
}

func headersMap(headers []agentfile.Header) map[string]string {
	result := map[string]string{}
	for _, header := range headers {
		result[header.Name] = header.ValueString()
	}
	return result
}

func tomlString(value string) string {
	return strconv.Quote(value)
}

func tomlStringArray(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, tomlString(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlInlineStringMap(values map[string]string) string {
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
		buffer.WriteString(tomlString(key))
		buffer.WriteString(" = ")
		buffer.WriteString(tomlString(values[key]))
	}
	buffer.WriteString(" }")
	return buffer.String()
}
