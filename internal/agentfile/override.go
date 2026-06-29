package agentfile

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// overrideFields is the single source of truth for spec fields that need
// special handling when applied as a run override. Each is an "exactly one of"
// union: the field-wise overlay would otherwise leave a stale sibling arm and
// fail validation, so reset clears the whole field before the overlay merges in
// the new arm. Asset fields additionally hold a Source, so a bare override
// value (no source key) is taken as a text source: --prompt "hi" => prompt.text.
var overrideFields = []struct {
	name  string
	asset bool
	reset func(*Spec)
}{
	{name: "llm", reset: func(s *Spec) { s.LLM = LLM{} }},
	{name: "prompt", asset: true, reset: func(s *Spec) { s.Prompt = nil }},
	{name: "systemPrompt", asset: true, reset: func(s *Spec) { s.SystemPrompt = nil }},
}

// AssetFields returns the spec field names whose value is a Source and that
// accept the bare-value text shorthand in overrides (e.g. prompt, systemPrompt).
func AssetFields() []string {
	var names []string
	for _, f := range overrideFields {
		if f.asset {
			names = append(names, f.name)
		}
	}
	return names
}

func (p *Project) ApplyOverride(path, value string) error {
	root, _, _ := strings.Cut(path, ".")
	for _, f := range overrideFields {
		if f.name != root {
			continue
		}
		if f.asset && path == root {
			path += ".text" // bare asset value => text source
		}
		f.reset(&p.AgentFile.Spec)
		break
	}
	if err := overlayField(&p.AgentFile.Spec, path, value); err != nil {
		return fmt.Errorf("unsupported field override %q: %w", path, err)
	}
	return p.AgentFile.Validate()
}

// overlayField decodes "a.b.c"=value as a one-field YAML layer over spec,
// reusing the same tag mapping the agentfile was parsed with. Decoding into a
// populated struct overlays field-wise (untouched fields keep their values),
// allocates pointers along the path, rejects unknown fields (KnownFields), and
// rejects list paths (a mapping cannot decode into a slice).
func overlayField(spec *Spec, path, value string) error {
	parts := strings.Split(path, ".")
	var node any = value
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "" {
			return fmt.Errorf("empty path segment")
		}
		node = map[string]any{parts[i]: node}
	}
	doc, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(doc))
	dec.KnownFields(true)
	return dec.Decode(spec)
}
