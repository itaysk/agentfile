package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
	"github.com/itaysk/agentfile/internal/registry"
)

func TestHelpDescribesBundlePrimaryHierarchy(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"build", "bundle build", "bundle run", "image build", "image run", "agents register"}},
		{[]string{"bundle", "--help"}, []string{"af bundle", "build", "run"}},
		{[]string{"image", "--help"}, []string{"af image", "build", "run"}},
		{[]string{"agents", "--help"}, []string{"af agents", "register", "remove"}},
		{[]string{"run", "--help"}, []string{"--bundle FILE", "--image REF", "--name NAME", "--acp"}},
		{[]string{"bundle", "run", "--help"}, []string{"af bundle run --bundle FILE", "--acp"}},
		{[]string{"image", "run", "--help"}, []string{"af image run --image REF"}},
		{[]string{"agents", "run", "--help"}, []string{"af agents run --name NAME"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != 0 {
			t.Fatalf("Run(%q) = %d, stderr = %q", tt.args, code, stderr.String())
		}
		for _, want := range tt.want {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("Run(%q) stdout = %q, want %q", tt.args, stdout.String(), want)
			}
		}
	}
}

func TestBundleBuildCanonicalAndNakedParity(t *testing.T) {
	dir := t.TempDir()
	agentfilePath := writeCLIAgentfile(t, dir, "build-parity", "1")
	naked := filepath.Join(dir, "naked.tar.gz")
	canonical := filepath.Join(dir, "canonical.tar.gz")

	for _, tt := range []struct {
		args   []string
		output string
	}{
		{[]string{"build", "--file", agentfilePath, "--output", naked}, naked},
		{[]string{"bundle", "build", "--file", agentfilePath, "--output", canonical}, canonical},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != 0 {
			t.Fatalf("Run(%q) = %d, stderr = %q", tt.args, code, stderr.String())
		}
		if stdout.String() != "Built "+tt.output+"\n" {
			t.Fatalf("Run(%q) stdout = %q", tt.args, stdout.String())
		}
	}
	nakedData, err := os.ReadFile(naked)
	if err != nil {
		t.Fatal(err)
	}
	canonicalData, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nakedData, canonicalData) {
		t.Fatal("canonical and naked bundle builds differ")
	}
}

func TestBundleBuildDefaultNameAndDockerIndependence(t *testing.T) {
	dir := t.TempDir()
	writeCLIAgentfile(t, dir, "default-name", "2")
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"build"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Built default-name__2.tar.gz\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "default-name__2.tar.gz")); err != nil {
		t.Fatal(err)
	}
}

func TestImageBuildRequiresBundleAndUsesTag(t *testing.T) {
	bundlePath := buildCLIBundle(t, "image-build", "3")
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"image", "build", "--bundle", bundlePath, "--base-image", "example/base:1", "--tag", "example/agent:3"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Built example/agent:3\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if log := readCLILog(t, logPath); !strings.Contains(log, "build -t example/agent:3") || !strings.Contains(log, "FROM example/base:1") {
		t.Fatalf("docker log = %q", log)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"image", "build", "--file", "agentfile.yaml"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "unknown image build argument") {
		t.Fatalf("removed image source build = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunSelectorsAreRequiredExclusiveAndScopedBeforeSideEffects(t *testing.T) {
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"run"}, "exactly one"},
		{[]string{"run", "--bundle", "missing", "--image", "example/image"}, "mutually exclusive"},
		{[]string{"bundle", "run", "--image", "example/image"}, "accepts only --bundle"},
		{[]string{"image", "run", "--bundle", "missing"}, "accepts only --image"},
		{[]string{"agents", "run", "--image", "example/image"}, "accepts only --name"},
		{[]string{"run", "positional-name"}, "does not accept positional"},
		{[]string{"run", "--file", "agentfile.yaml"}, "unknown run argument"},
		{[]string{"build", "--target", "image"}, "unknown bundle build argument"},
		{[]string{"agents", "remove", "legacy-name"}, "does not accept positional"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != 1 {
			t.Fatalf("Run(%q) = %d, stderr = %q", tt.args, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), tt.want) {
			t.Fatalf("Run(%q) stderr = %q, want %q", tt.args, stderr.String(), tt.want)
		}
	}
	if log := readCLILog(t, logPath); log != "" {
		t.Fatalf("invalid selectors caused Docker side effects:\n%s", log)
	}
}

func TestBundleRunCanonicalNakedAndRegisteredParity(t *testing.T) {
	isolateCLIConfig(t)
	bundlePath := buildCLIBundle(t, "bundle-run", "1")
	binDir := installCLIFakeHarness(t)
	t.Setenv("PATH", binDir)

	var registerOut, registerErr bytes.Buffer
	if code := Run([]string{"agents", "register", "--name", "friendly", "--bundle", bundlePath}, &registerOut, &registerErr); code != 0 {
		t.Fatalf("register = %d, stderr = %q", code, registerErr.String())
	}
	if registerOut.String() != "Registered friendly\n" {
		t.Fatalf("register stdout = %q", registerOut.String())
	}
	if err := os.Remove(bundlePath); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"run", "--name", "friendly"},
		{"agents", "run", "--name", "friendly"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("Run(%q) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.String() != "cli-harness\n" || !strings.Contains(stderr.String(), "without isolation") {
			t.Fatalf("Run(%q) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}

	managed := registeredBundlePath(t, "friendly")
	for _, args := range [][]string{
		{"run", "--bundle", managed},
		{"bundle", "run", "--bundle", managed},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 0 || stdout.String() != "cli-harness\n" {
			t.Fatalf("Run(%q) = %d, stdout = %q, stderr = %q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestImageRunCanonicalNakedAndRegisteredParity(t *testing.T) {
	isolateCLIConfig(t)
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"agents", "register", "--name", "friendly", "--image", "acme/agent:1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("register = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Registered friendly\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"agents", "register", "--image", "acme/inferred:1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inferred register = %d, stderr = %q", code, stderr.String())
	}
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	if entry := reg.Agents["image-agent"]; entry.Image != "acme/inferred:1" || entry.Version != "latest" || entry.Harness != "codex" || entry.Digest != "sha256:0123456789abcdef" {
		t.Fatalf("inferred image entry = %#v", reg.Agents["image-agent"])
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"run", "--image", "acme/agent:1", "--prompt", "hi"},
		{"image", "run", "--image", "acme/agent:1", "--prompt", "hi"},
		{"run", "--name", "friendly", "--prompt", "hi"},
		{"agents", "run", "--name", "friendly", "--prompt", "hi"},
	} {
		var runOut, runErr bytes.Buffer
		if code := Run(args, &runOut, &runErr); code != 0 {
			t.Fatalf("Run(%q) = %d, stderr = %q", args, code, runErr.String())
		}
	}
	if got := strings.Count(readCLILog(t, logPath), "run --rm"); got != 4 {
		t.Fatalf("docker run count = %d, want 4\n%s", got, readCLILog(t, logPath))
	}
}

func TestImageRunPullsBeforeRetryingInspection(t *testing.T) {
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))
	t.Setenv("DOCKER_INSPECT_FAIL_ONCE", filepath.Join(t.TempDir(), "failed"))
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--image", "remote/agent:1", "--prompt", "hi"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "pulling remote/agent:1") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	log := readCLILog(t, logPath)
	if strings.Count(log, "image inspect") != 2 || !strings.Contains(log, "pull remote/agent:1") {
		t.Fatalf("docker log = %q", log)
	}
}

func TestRunFlagsReachImageRunner(t *testing.T) {
	dockerPath, logPath := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))
	t.Setenv("GITHUB_TOKEN", "host-token")
	workspace := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(envFile, []byte("FROM_FILE=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"run", "--image", "acme/agent:1", "--prompt", "say hi", "--model", "gpt-5",
		"--workspace", workspace, "--env", "EXTRA=value", "--env-file", envFile, "--env-auto", "--debug",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, stderr = %q", code, stderr.String())
	}
	log := readCLILog(t, logPath)
	for _, want := range []string{
		"-e AGENTFILE_MODEL=gpt-5", "-e AGENTFILE_PROMPT=say hi", "-e EXTRA=value", "-e GITHUB_TOKEN=host-token",
		"--env-file " + envFile, "--mount type=bind,source=" + workspace + ",target=/agent/workspace", "acme/agent:1",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker log = %q, want %q", log, want)
		}
	}
}

func TestBundleACPAcceptedDirectAndThroughRegistry(t *testing.T) {
	isolateCLIConfig(t)
	bundlePath := buildCLIBundle(t, "bundle-acp", "1")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--bundle", bundlePath, "--acp"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "without isolation") {
		t.Fatalf("direct ACP = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"agents", "register", "--bundle", bundlePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("register = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"run", "--name", "bundle-acp", "--acp"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "without isolation") {
		t.Fatalf("registered ACP = %d, stderr = %q", code, stderr.String())
	}
}

func TestRegisterBundleInferenceOverrideListAndRemove(t *testing.T) {
	isolateCLIConfig(t)
	bundlePath := buildCLIBundle(t, "inferred", "1")

	for _, args := range [][]string{
		{"agents", "register", "--bundle", bundlePath},
		{"agents", "register", "--name", "override", "--bundle", bundlePath},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("Run(%q) = %d, stderr = %q", args, code, stderr.String())
		}
	}
	managedPath := registeredBundlePath(t, "inferred")
	entries, err := os.ReadDir(filepath.Dir(managedPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("managed bundles = %d, want deduplicated one", len(entries))
	}

	var listOut, listErr bytes.Buffer
	if code := Run([]string{"agents", "list"}, &listOut, &listErr); code != 0 {
		t.Fatalf("list = %d, stderr = %q", code, listErr.String())
	}
	lines := strings.Split(strings.TrimSpace(listOut.String()), "\n")
	if got := strings.Join(strings.Fields(lines[0]), ","); got != "NAME,VERSION,HARNESS,DIGEST" {
		t.Fatalf("list header = %q", lines[0])
	}
	wantDigest := strings.TrimSuffix(filepath.Base(managedPath), ".tar.gz")[:12]
	if fields := strings.Fields(lines[1]); len(fields) != 4 || fields[0] != "inferred" || fields[1] != "1" || fields[2] != "codex" || fields[3] != wantDigest {
		t.Fatalf("list = %q, want inferred agent metadata", listOut.String())
	}
	if !strings.Contains(listOut.String(), "override") {
		t.Fatalf("list = %q, want override", listOut.String())
	}

	for _, name := range []string{"inferred", "override"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"agents", "remove", "--name", name}, &stdout, &stderr); code != 0 {
			t.Fatalf("remove %s = %d, stderr = %q", name, code, stderr.String())
		}
	}
	entries, err = os.ReadDir(registeredBundlePathDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("managed bundles remain after last removal: %v", entries)
	}
}

func TestListShowsUnknownMetadataForExistingRegistryEntries(t *testing.T) {
	isolateCLIConfig(t)
	reg := &registry.Registry{Agents: map[string]registry.Entry{
		"existing": {Name: "existing", Bundle: "bundle.tar.gz"},
	}}
	if err := registry.Save(reg); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runList(nil, &stdout); err != nil {
		t.Fatal(err)
	}
	if fields := strings.Fields(strings.Split(stdout.String(), "\n")[1]); len(fields) != 4 || strings.Join(fields[1:], "") != "---" {
		t.Fatalf("list = %q, want unknown metadata markers", stdout.String())
	}
}

func TestReplacingRegisteredBundleRemovesOldManagedCopy(t *testing.T) {
	isolateCLIConfig(t)
	first := buildCLIBundle(t, "first", "1")
	second := buildCLIBundle(t, "second", "1")
	for _, source := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"agents", "register", "--name", "same", "--bundle", source}, &stdout, &stderr); code != 0 {
			t.Fatalf("register %s = %d, stderr = %q", source, code, stderr.String())
		}
	}
	entries, err := os.ReadDir(registeredBundlePathDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Join(registeredBundlePathDir(t), entries[0].Name()) != registeredBundlePath(t, "same") {
		t.Fatalf("managed bundles after replacement = %v", entries)
	}
}

func TestRegisterRejectsMalformedBundleAndNonAgentfileImage(t *testing.T) {
	isolateCLIConfig(t)
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(bad, []byte("not a bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"agents", "register", "--bundle", bad}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "bundle gzip") {
		t.Fatalf("malformed register = %d, stderr = %q", code, stderr.String())
	}

	dockerPath, _ := installCLIFakeDocker(t)
	t.Setenv("PATH", filepath.Dir(dockerPath))
	t.Setenv("DOCKER_BAD_IMAGE", "1")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"agents", "register", "--image", "plain:latest"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "was not built by agentfile") {
		t.Fatalf("plain image register = %d, stderr = %q", code, stderr.String())
	}
}

func TestParseRunFlagsModesAndWorkspace(t *testing.T) {
	options := runFlags{}
	if err := parseRunFlags([]string{"--image", "agent:1", "--tui"}, &options, imageSelector); err != nil {
		t.Fatal(err)
	}
	if options.mode != harness.ModeTUI {
		t.Fatalf("mode = %q", options.mode)
	}
	for _, args := range [][]string{
		{"--image", "agent:1", "--tui", "--prompt", "hi"},
		{"--image", "agent:1", "--acp", "--workspace", "."},
		{"--image", "agent:1", "--acp", "--tui"},
	} {
		if err := parseRunFlags(args, &runFlags{}, allSelectors); err == nil {
			t.Fatalf("parseRunFlags(%q) succeeded", args)
		}
	}
}

func TestRunErrorPrefixesOnce(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"bundle", "unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if strings.Count(stderr.String(), "af:") != 1 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func writeCLIAgentfile(t *testing.T, dir, name, version string) string {
	t.Helper()
	path := filepath.Join(dir, "agentfile.yaml")
	content := `apiVersion: agentfile.build/v1
kind: Agent
metadata:
  name: ` + name + `
  version: "` + version + `"
spec:
  harness:
    codex: {}
  llm:
    openai:
      model: gpt-5-mini
  prompt:
    text: say hi
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildCLIBundle(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	project, err := agentfile.Load(writeCLIAgentfile(t, dir, name, version))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".tar.gz")
	if err := bundle.Build(project, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func isolateCLIConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
}

func registeredBundlePath(t *testing.T, name string) string {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	return reg.Agents[name].Bundle
}

func registeredBundlePathDir(t *testing.T) string {
	t.Helper()
	path, err := registry.Path()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(path), "bundles")
}

func installCLIFakeHarness(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho cli-harness\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func installCLIFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_ARGS_LOG"
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  if [ -n "${DOCKER_INSPECT_FAIL_ONCE:-}" ] && [ ! -f "$DOCKER_INSPECT_FAIL_ONCE" ]; then
    : > "$DOCKER_INSPECT_FAIL_ONCE"
    exit 1
  fi
  if [ -n "${DOCKER_BAD_IMAGE:-}" ]; then
    echo '{}'
    exit 0
  fi
  printf '%s\n' '{"build.agentfile.metadata":"{\"name\":\"image-agent\",\"version\":\"latest\"}","build.agentfile.runtimeEnv":"[\"GITHUB_TOKEN\"]","build.agentfile.harness":"codex","build.agentfile.bundle.digest":"sha256:0123456789abcdef"}'
  exit 0
fi
if [ "$1" = "pull" ]; then
  echo "pulling $2" >&2
  exit 0
fi
if [ "$1" = "build" ]; then
  for last do :; done
  IFS= read -r first < "$last/Dockerfile"
  printf '%s\n' "$first" >> "$DOCKER_ARGS_LOG"
  exit 0
fi
if [ "$1" = "run" ]; then
  if [ -n "${DOCKER_AGENT_STDERR:-}" ]; then
    echo "$DOCKER_AGENT_STDERR" >&2
  fi
  if [ -n "${DOCKER_AGENT_EXIT:-}" ]; then
    exit "$DOCKER_AGENT_EXIT"
  fi
fi
exit 0
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_ARGS_LOG", logPath)
	return dockerPath, logPath
}

func readCLILog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
