package registry

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/bundle"
)

func TestImportBundleValidatesCopiesAndDeduplicates(t *testing.T) {
	isolateConfig(t)
	source := filepath.Join(t.TempDir(), "agent.tar.gz")
	writeTestBundle(t, source, "hello")
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(sourceBytes)

	managed, manifest, err := ImportBundle(source)
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := bundlesPath()
	if err != nil {
		t.Fatal(err)
	}
	wantPath = filepath.Join(wantPath, hex.EncodeToString(wantHash[:])+".tar.gz")
	if managed != wantPath || manifest.Agent.Name != "hello" {
		t.Fatalf("ImportBundle = %q, %q; want %q, hello", managed, manifest.Agent.Name, wantPath)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := bundle.Extract(managed, filepath.Join(t.TempDir(), "extracted")); err != nil {
		t.Fatalf("extract managed bundle after source removal: %v", err)
	}

	duplicate := filepath.Join(t.TempDir(), "duplicate.tar.gz")
	if err := os.WriteFile(duplicate, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	again, _, err := ImportBundle(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if again != managed {
		t.Fatalf("duplicate bundle path = %q, want %q", again, managed)
	}
	entries, err := os.ReadDir(filepath.Dir(managed))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("managed bundle files = %d, want 1", len(entries))
	}
}

func TestImportBundleRejectsMalformedArchive(t *testing.T) {
	isolateConfig(t)
	source := filepath.Join(t.TempDir(), "broken.tar.gz")
	if err := os.WriteFile(source, []byte("not a bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ImportBundle(source); err == nil {
		t.Fatal("ImportBundle accepted malformed archive")
	}
	dir, err := bundlesPath()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("bundle storage contains %d files after failed import, want 0", len(entries))
	}
}

func TestCleanupBundlesRemovesOnlyUnreferencedManagedCopies(t *testing.T) {
	isolateConfig(t)
	firstSource := filepath.Join(t.TempDir(), "first.tar.gz")
	secondSource := filepath.Join(t.TempDir(), "second.tar.gz")
	writeTestBundle(t, firstSource, "first")
	writeTestBundle(t, secondSource, "second")
	first, _, err := ImportBundle(firstSource)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := ImportBundle(secondSource)
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(filepath.Dir(first), "notes.txt")
	if err := os.WriteFile(notes, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{}
	reg.Register(Entry{Name: "agent", Bundle: first})
	if err := CleanupBundles(reg); err != nil {
		t.Fatal(err)
	}
	assertExists(t, first, true)
	assertExists(t, second, false)

	second, _, err = ImportBundle(secondSource)
	if err != nil {
		t.Fatal(err)
	}
	reg.Register(Entry{Name: "agent", Bundle: second})
	reg.Register(Entry{Name: "shared", Bundle: second})
	if err := CleanupBundles(reg); err != nil {
		t.Fatal(err)
	}
	assertExists(t, first, false)
	assertExists(t, second, true)
	reg.Remove("agent")
	if err := CleanupBundles(reg); err != nil {
		t.Fatal(err)
	}
	assertExists(t, second, true)
	reg.Remove("shared")
	if err := CleanupBundles(reg); err != nil {
		t.Fatal(err)
	}
	assertExists(t, second, false)
	assertExists(t, notes, true)
}

func TestRegistryRequiresExactlyOneBundleOrImage(t *testing.T) {
	isolateConfig(t)
	valid := &Registry{Agents: map[string]Entry{
		"bundle-agent": {Name: "bundle-agent", Bundle: "/managed/bundle.tar.gz"},
		"image-agent":  {Name: "image-agent", Image: "example/agent:latest"},
	}}
	if err := Save(valid); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mustRegistryPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "agentfilePath") || !strings.Contains(string(data), `"bundle"`) || !strings.Contains(string(data), `"image"`) {
		t.Fatalf("registry JSON = %s", data)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Agents) != 2 {
		t.Fatalf("loaded entries = %d, want 2", len(loaded.Agents))
	}

	for _, entry := range []Entry{
		{Name: "missing"},
		{Name: "both", Bundle: "bundle.tar.gz", Image: "image:latest"},
	} {
		reg := &Registry{Agents: map[string]Entry{entry.Name: entry}}
		if err := Save(reg); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("Save(%+v) error = %v, want exactly-one error", entry, err)
		}
	}

	legacy := `{"agents":{"old":{"name":"old","agentfilePath":"agentfile.yaml"}}}`
	if err := os.WriteFile(mustRegistryPath(t), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("Load legacy registry error = %v, want exactly-one error", err)
	}
}

func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
}

func mustRegistryPath(t *testing.T) string {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestBundle(t *testing.T, filename, name string) {
	t.Helper()
	manifest := bundle.Manifest{
		BundleVersion: bundle.Version,
		Agent:         bundle.Agent{Name: name, Version: "latest"},
		Harness:       "pi",
		Model:         bundle.Model{Provider: "openai", Name: "gpt-5"},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: bundle.ManifestName, Mode: 0o644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if (err == nil) != want || err != nil && !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%q) error = %v, want exists %t", path, err, want)
	}
}
