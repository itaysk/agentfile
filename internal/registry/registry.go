package registry

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/itaysk/agentfile/internal/bundle"
)

const appDirName = "agentfile"

type Registry struct {
	Agents map[string]Entry `json:"agents"`
}

type Entry struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Harness string `json:"harness,omitempty"`
	Digest  string `json:"digest,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
	Image   string `json:"image,omitempty"`
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
	if err := registry.validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}

// Save replaces the user-local registry with reg.
func Save(reg *Registry) error {
	if err := reg.validate(); err != nil {
		return err
	}
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

// ImportBundle validates source and copies it into content-addressed storage.
func ImportBundle(source string) (string, bundle.Manifest, error) {
	dir, err := bundlesPath()
	if err != nil {
		return "", bundle.Manifest{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", bundle.Manifest{}, err
	}
	temp, err := os.CreateTemp(dir, ".import-*")
	if err != nil {
		return "", bundle.Manifest{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	src, err := os.Open(source)
	if err != nil {
		temp.Close()
		return "", bundle.Manifest{}, err
	}
	_, copyErr := io.Copy(temp, src)
	srcErr := src.Close()
	closeErr := temp.Close()
	if copyErr != nil {
		return "", bundle.Manifest{}, copyErr
	}
	if srcErr != nil {
		return "", bundle.Manifest{}, srcErr
	}
	if closeErr != nil {
		return "", bundle.Manifest{}, closeErr
	}

	extractDir, err := os.MkdirTemp("", "agentfile-registry-bundle-*")
	if err != nil {
		return "", bundle.Manifest{}, err
	}
	defer os.RemoveAll(extractDir)
	unpacked, err := bundle.Extract(tempPath, extractDir)
	if err != nil {
		return "", bundle.Manifest{}, err
	}
	hash := strings.TrimPrefix(unpacked.Digest, "sha256:")
	destination := filepath.Join(dir, hash+".tar.gz")
	if _, err := os.Stat(destination); err == nil {
		return destination, unpacked.Manifest, nil
	} else if !os.IsNotExist(err) {
		return "", bundle.Manifest{}, err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return "", bundle.Manifest{}, err
	}
	return destination, unpacked.Manifest, nil
}

// CleanupBundles removes managed bundles that are not referenced by reg.
func CleanupBundles(reg *Registry) error {
	if reg == nil {
		return fmt.Errorf("registry is required")
	}
	dir, err := bundlesPath()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	referenced := map[string]struct{}{}
	for _, entry := range reg.Agents {
		if entry.Bundle != "" {
			referenced[filepath.Clean(entry.Bundle)] = struct{}{}
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || !managedBundleName(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if _, ok := referenced[path]; ok {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
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

func bundlesPath() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "bundles"), nil
}

func managedBundleName(name string) bool {
	hash := strings.TrimSuffix(name, ".tar.gz")
	if hash == name || len(hash) != sha256HexLength {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func (r *Registry) validate() error {
	if r == nil {
		return fmt.Errorf("registry is required")
	}
	for name, entry := range r.Agents {
		if entry.Name == "" {
			return fmt.Errorf("registry entry %q is missing name", name)
		}
		if entry.Name != name {
			return fmt.Errorf("registry entry %q has name %q", name, entry.Name)
		}
		if (entry.Bundle == "") == (entry.Image == "") {
			return fmt.Errorf("registry entry %q must contain exactly one of bundle or image", name)
		}
	}
	return nil
}

const sha256HexLength = 64
