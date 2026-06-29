package config

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
	Name            string `json:"name"`
	ProjectDir      string `json:"projectDir"`
	AgentfilePath   string `json:"agentfilePath"`
	DefaultImageTag string `json:"defaultImageTag"`
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

func RegistryPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "registry.json"), nil
}

func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
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

func SaveRegistry(registry *Registry) error {
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (r *Registry) Put(entry Entry) {
	if r.Agents == nil {
		r.Agents = map[string]Entry{}
	}
	r.Agents[entry.Name] = entry
}

func (r *Registry) Remove(name string) bool {
	if _, ok := r.Agents[name]; !ok {
		return false
	}
	delete(r.Agents, name)
	return true
}

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
