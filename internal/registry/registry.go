package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const appDirName = "agentfile"

type Registry struct {
	Agents map[string]Entry `json:"agents"`
}

type Entry struct {
	Name          string `json:"name"`
	AgentfilePath string `json:"agentfilePath,omitempty"`
	ImageRef      string `json:"image,omitempty"`
}

// Path returns the user-local registry file path.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName, "registry.json"), nil
}

// Load reads the user-local registry, returning an empty registry if it does
// not yet exist.
func Load() (*Registry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Agents: map[string]Entry{}}, nil
		}
		return nil, err
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	if registry.Agents == nil {
		registry.Agents = map[string]Entry{}
	}
	return &registry, nil
}

// Save replaces the user-local registry with reg.
func Save(reg *Registry) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Register adds or replaces an entry by name.
func (r *Registry) Register(entry Entry) {
	if r.Agents == nil {
		r.Agents = map[string]Entry{}
	}
	r.Agents[entry.Name] = entry
}

// Remove deletes name and reports whether it existed.
func (r *Registry) Remove(name string) bool {
	if _, ok := r.Agents[name]; !ok {
		return false
	}
	delete(r.Agents, name)
	return true
}

// SortedEntries returns a copy of the entries ordered by name.
func (r *Registry) SortedEntries() []Entry {
	entries := make([]Entry, 0, len(r.Agents))
	for _, entry := range r.Agents {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}
