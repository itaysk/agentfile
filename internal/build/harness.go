package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/itaysk/agentfile/internal/agentfile"
)

// configFile is harness config content destined for an absolute in-container
// path. It is staged into the image at that path as-is — with placeholder
// tokens where runtime values go — and the generated entrypoint substitutes
// the tokens in place at container start. A file without runtime references
// is final as staged.
type configFile struct {
	path    string
	content string
}

func refToken(name string) string {
	return agentfile.RefTokenPrefix + name + "__"
}

func harnessConfigFiles(af agentfile.AgentFile, assets *agentfile.ResolvedAssets) ([]configFile, error) {
	switch af.Spec.Harness.Name() {
	case "claudecode":
		return claudeCodeConfigFiles(af)
	case "codex":
		return codexConfigFiles(af, assets), nil
	case "pi":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", af.Spec.Harness.Name())
	}
}

func claudeCodeConfigFiles(af agentfile.AgentFile) ([]configFile, error) {
	if len(af.Spec.MCPs) == 0 {
		return nil, nil
	}
	config := map[string]any{
		"mcpServers": claudeMCPServers(af.Spec.MCPs),
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return []configFile{{path: "/agent/agentfile/claudecode/mcp.json", content: string(data) + "\n"}}, nil
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
					envs[env.Name] = configValue(env.ValueSource)
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
				headers[header.Name] = configValue(header.ValueSource)
			}
			server["headers"] = headers
		}
		servers[mcp.Name] = server
	}
	return servers
}

// configValue renders a value source into harness config content: literals
// verbatim, runtime sources as placeholder tokens the entrypoint expands.
// New value-source kinds get their branch here.
func configValue(v agentfile.ValueSource) string {
	if v.RuntimeEnv != nil {
		return refToken(v.RuntimeEnv.Name)
	}
	return v.ValueString()
}

func codexConfigFiles(af agentfile.AgentFile, assets *agentfile.ResolvedAssets) []configFile {
	var builder strings.Builder
	builder.WriteString("project_doc_max_bytes = 0\n")
	if assets.HasSystemPrompt {
		builder.WriteString("model_instructions_file = ")
		builder.WriteString(tomlString("/agent/agentfile/system-prompt.md"))
		builder.WriteString("\n")
	}
	builder.WriteString("\n[projects.\"/agent/workspace\"]\ntrust_level = \"trusted\"\n")
	if len(af.Spec.MCPs) > 0 {
		builder.WriteString("\n")
		writeCodexMCPConfig(&builder, af.Spec.MCPs)
	}
	return []configFile{{path: "/agent/agentfile/codex/home/.codex/config.toml", content: builder.String()}}
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
					builder.WriteString(tomlString(configValue(env.ValueSource)))
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
		result[header.Name] = configValue(header.ValueSource)
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
