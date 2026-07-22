package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/itaysk/agentfile/internal/agentfile"
	"github.com/itaysk/agentfile/internal/bundle"
	"github.com/itaysk/agentfile/internal/harness"
	"github.com/itaysk/agentfile/internal/registry"
	"github.com/itaysk/agentfile/internal/runner"
)

// version is set via -ldflags
var version = "dev"

// Run runs af with args and returns its exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintln(stdout, version)
		return 0
	}

	code := 0
	var err error
	switch args[0] {
	case "build":
		err = runBundleBuild(args[1:], stdout, "af build")
	case "run":
		code, err = runAgent(args[1:], stdout, stderr, allSelectors, "af run")
	case "ps":
		err = runPS(args[1:], stdout)
	case "bundle":
		code, err = runBundleCommands(args[1:], stdout, stderr)
	case "image":
		code, err = runImageCommands(args[1:], stdout, stderr)
	case "agents":
		code, err = runAgents(args[1:], stdout, stderr)
	default:
		code, err = 1, fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(stderr, "af:", err)
		if code == 0 {
			return 1
		}
	}
	return code
}

func runBundleCommands(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printBundleHelp(stdout)
		return 0, nil
	}
	switch args[0] {
	case "build":
		return 0, runBundleBuild(args[1:], stdout, "af bundle build")
	case "run":
		return runAgent(args[1:], stdout, stderr, bundleSelector, "af bundle run")
	default:
		return 1, fmt.Errorf("unknown bundle command %q", args[0])
	}
}

func runImageCommands(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printImageHelp(stdout)
		return 0, nil
	}
	switch args[0] {
	case "build":
		return 0, runImageBuild(args[1:], stdout, stderr)
	case "run":
		return runAgent(args[1:], stdout, stderr, imageSelector, "af image run")
	default:
		return 1, fmt.Errorf("unknown image command %q", args[0])
	}
}

func runBundleBuild(args []string, stdout io.Writer, command string) error {
	if wantsHelp(args) {
		fmt.Fprintf(stdout, "usage: %s [--file FILE] [--output FILE]\n", command)
		return nil
	}
	options := bundleBuildFlags{file: agentfile.DefaultFileName}
	if err := parseBundleBuildFlags(args, &options); err != nil {
		return err
	}
	project, err := agentfile.Load(options.file)
	if err != nil {
		return err
	}
	output := options.output
	if output == "" {
		output = bundle.DefaultFilename(project.AgentFile.Metadata)
	}
	if err := bundle.Build(project, output); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Built %s\n", output)
	return nil
}

func runImageBuild(args []string, stdout, stderr io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af image build --bundle FILE [--base-image REF] [--tag TAG]")
		return nil
	}
	options := imageBuildFlags{}
	if err := parseImageBuildFlags(args, &options); err != nil {
		return err
	}
	tag, err := runner.BuildImage(context.Background(), runner.BuildImageOptions{
		BundlePath: options.bundle,
		BaseImage:  options.baseImage,
		Tag:        options.tag,
		Stdout:     stdout,
		Stderr:     stderr,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Built %s\n", tag)
	return nil
}

func runAgents(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printAgentsHelp(stdout)
		return 0, nil
	}
	var err error
	switch args[0] {
	case "run":
		return runAgent(args[1:], stdout, stderr, nameSelector, "af agents run")
	case "register":
		err = runRegister(args[1:], stdout)
	case "list":
		err = runList(args[1:], stdout)
	case "remove":
		err = runRemove(args[1:], stdout)
	default:
		err = fmt.Errorf("unknown agents command %q", args[0])
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

func runAgent(args []string, stdout, stderr io.Writer, allowed runSelector, command string) (int, error) {
	if wantsHelp(args) {
		modes := "--tui | --acp | --prompt TEXT"
		fmt.Fprintf(stdout, "usage: %s %s [%s] [--model MODEL] [--workspace DIR | --ws DIR] [--env KEY[=VALUE]] [--env-file FILE] [--env-auto] [--debug]\n", command, selectorUsage(allowed), modes)
		return 0, nil
	}
	options := runFlags{env: map[string]string{}}
	if err := parseRunFlags(args, &options, allowed); err != nil {
		return 1, err
	}
	runStderr := io.Discard
	var failureStderr io.Writer = stderr
	if options.debug || options.mode != "" {
		runStderr = stderr
		failureStderr = nil
	}
	// Pull progress goes to real stderr even without --debug: a first pull can
	// take minutes and stdout stays clean either way.
	runOptions, err := selectRunInput(options, stderr)
	if err != nil {
		return 1, err
	}
	runOptions.Prompt = options.prompt
	runOptions.Model = options.model
	runOptions.Env = options.env
	runOptions.EnvFiles = options.envFiles
	runOptions.InheritRuntimeEnv = options.envAuto
	runOptions.Workspace = options.workspace
	runOptions.Mode = options.mode
	runOptions.Stdout = stdout
	runOptions.Stderr = runStderr
	runOptions.FailureStderr = failureStderr
	runOptions.WarningStderr = stderr
	agent, runtime := options.bundle, "bundle"
	if options.image != "" {
		agent, runtime = options.image, "image"
	} else if options.name != "" {
		agent = options.name
		if runOptions.ImageRef != "" {
			runtime = "image"
		}
	}
	mode := string(options.mode)
	if options.mode == "" || options.mode == harness.ModeOneShot {
		mode = "one-shot"
	}
	stopTracking, err := trackRun(runningAgent{
		PID:     os.Getpid(),
		Agent:   agent,
		Runtime: runtime,
		Mode:    mode,
		Started: time.Now().UTC(),
	})
	if err != nil {
		return 1, fmt.Errorf("track running agent: %w", err)
	}
	defer stopTracking()
	return runner.Run(context.Background(), runOptions)
}

func runRegister(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af agents register [--name NAME] (--bundle FILE | --image REF)")
		return nil
	}
	options := registerFlags{}
	if err := parseRegisterFlags(args, &options); err != nil {
		return err
	}
	name := options.name
	entry := registry.Entry{Image: options.image}
	if options.image != "" {
		info, err := runner.ReadImageInfo(context.Background(), "", options.image)
		if err != nil {
			return fmt.Errorf("%w (docker pull the image first if it is not local)", err)
		}
		entry.Version = info.Metadata.Version
		entry.Harness = info.HarnessName
		entry.Digest = info.BundleDigest
		if name == "" {
			name = info.Metadata.Name
		}
	} else {
		managedPath, manifest, err := registry.ImportBundle(options.bundle)
		if err != nil {
			return err
		}
		entry.Bundle = managedPath
		entry.Version = manifest.Agent.Version
		entry.Harness = manifest.Harness
		entry.Digest = "sha256:" + strings.TrimSuffix(filepath.Base(managedPath), ".tar.gz")
		if name == "" {
			name = manifest.Agent.Name
		}
	}
	entry.Name = name
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	reg.Register(entry)
	if err := registry.Save(reg); err != nil {
		return err
	}
	if err := registry.CleanupBundles(reg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Registered %s\n", name)
	return nil
}

func runList(args []string, stdout io.Writer) error {
	if len(args) > 0 {
		if args[0] == "--help" || args[0] == "-h" {
			fmt.Fprintln(stdout, "usage: af agents list")
			return nil
		}
		return fmt.Errorf("agents list does not accept arguments")
	}
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tVERSION\tHARNESS\tDIGEST")
	for _, entry := range reg.SortedEntries() {
		version := entry.Version
		if version == "" {
			version = "-"
		}
		harness := entry.Harness
		if harness == "" {
			harness = "-"
		}
		digest := strings.TrimPrefix(entry.Digest, "sha256:")
		digest = digest[:min(len(digest), 12)]
		if digest == "" {
			digest = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", entry.Name, version, harness, digest)
	}
	return writer.Flush()
}

func runRemove(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af agents remove --name NAME")
		return nil
	}
	name, err := parseRequiredName(args, "remove")
	if err != nil {
		return err
	}
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	if !reg.Remove(name) {
		return fmt.Errorf("agent %q is not registered", name)
	}
	if err := registry.Save(reg); err != nil {
		return err
	}
	if err := registry.CleanupBundles(reg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %s\n", name)
	return nil
}

func selectRunInput(flags runFlags, progress io.Writer) (runner.Options, error) {
	if flags.bundle != "" {
		return runner.Options{BundlePath: flags.bundle}, nil
	}
	if flags.image != "" {
		info, err := readOrPullImageInfo(flags.image, progress)
		if err != nil {
			return runner.Options{}, err
		}
		return runner.Options{ImageRef: flags.image, RuntimeEnvNames: info.RuntimeEnvNames, HarnessName: info.HarnessName}, nil
	}
	if flags.name != "" {
		reg, err := registry.Load()
		if err != nil {
			return runner.Options{}, err
		}
		entry, ok := reg.Agents[flags.name]
		if !ok {
			return runner.Options{}, fmt.Errorf("agent %q is not registered", flags.name)
		}
		if entry.Image != "" {
			info, err := readOrPullImageInfo(entry.Image, progress)
			if err != nil {
				return runner.Options{}, err
			}
			return runner.Options{ImageRef: entry.Image, RuntimeEnvNames: info.RuntimeEnvNames, HarnessName: info.HarnessName}, nil
		}
		return runner.Options{BundlePath: entry.Bundle}, nil
	}
	return runner.Options{}, fmt.Errorf("run selector is required")
}

func readOrPullImageInfo(imageRef string, progress io.Writer) (*runner.ImageInfo, error) {
	ctx := context.Background()
	info, err := runner.ReadImageInfo(ctx, "", imageRef)
	if err != nil {
		if err := runner.PullImage(ctx, "", imageRef, progress); err != nil {
			return nil, err
		}
		if info, err = runner.ReadImageInfo(ctx, "", imageRef); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af COMMAND [ARGS]

Commands:
  build              build a bundle (alias for af bundle build)
  run                run a bundle, image, or registered agent
  ps                 list running agents
  bundle build       build a bundle
  bundle run         run a bundle
  image build        build an image from a bundle
  image run          run an image
  agents run         run a registered agent
  agents register    register a bundle or image
  agents list        list registered agents
  agents remove      remove a registered agent
  version            print the af version

Use "af bundle --help", "af image --help", or "af agents --help" for details.`)
}

func printBundleHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af bundle COMMAND [ARGS]

Commands:
  build              build a bundle from an agentfile
  run                run a bundle`)
}

func printImageHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af image COMMAND [ARGS]

Commands:
  build              build an image from a bundle
  run                run an image`)
}

func printAgentsHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af agents COMMAND [ARGS]

Commands:
  run                run a registered agent
  register           register a bundle or image
  list               list registered agents
  remove             remove a registered agent`)
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

type runSelector uint8

const (
	bundleSelector runSelector = 1 << iota
	imageSelector
	nameSelector
	allSelectors = bundleSelector | imageSelector | nameSelector
)

type bundleBuildFlags struct {
	file   string
	output string
}

type imageBuildFlags struct {
	bundle    string
	tag       string
	baseImage string
}

type registerFlags struct {
	bundle string
	image  string
	name   string
}

type runFlags struct {
	name      string
	image     string
	bundle    string
	env       map[string]string
	envFiles  []string
	envAuto   bool
	workspace string
	prompt    *string
	model     string
	debug     bool
	mode      harness.Mode
}

// matchValueFlag recognizes "--long value", "short value", "--long=value", and "short=value".
// matched is false when arg is not this flag; err is non-nil only when a value
// is required but missing.
func matchValueFlag(args []string, i int, arg, long, short string) (value string, next int, matched bool, err error) {
	switch {
	case arg == long || (short != "" && arg == short):
		v, n, e := consumeValue(args, i, arg)
		return v, n, true, e
	case strings.HasPrefix(arg, long+"="):
		return strings.TrimPrefix(arg, long+"="), i, true, nil
	case short != "" && strings.HasPrefix(arg, short+"="):
		return strings.TrimPrefix(arg, short+"="), i, true, nil
	}
	return "", i, false, nil
}

func parseBundleBuildFlags(args []string, flags *bundleBuildFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--file requires a value")
			}
			flags.file, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--output", "-o"); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--output requires a value")
			}
			flags.output, i = value, next
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown bundle build argument %q", arg)
		}
		return fmt.Errorf("bundle build does not accept positional arguments")
	}
	return nil
}

func parseImageBuildFlags(args []string, flags *imageBuildFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--bundle", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--bundle requires a value")
			}
			flags.bundle, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--tag", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--tag requires a value")
			}
			flags.tag, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--base-image", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--base-image requires a value")
			}
			flags.baseImage, i = value, next
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown image build argument %q", arg)
		}
		return fmt.Errorf("image build does not accept positional arguments")
	}
	if flags.bundle == "" {
		return fmt.Errorf("--bundle is required")
	}
	return nil
}

func parseRegisterFlags(args []string, flags *registerFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--name", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--name requires a value")
			}
			flags.name, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--bundle", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--bundle requires a value")
			}
			flags.bundle, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--image", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				flag, _, _ := strings.Cut(arg, "=")
				return fmt.Errorf("%s requires a value", flag)
			}
			flags.image, i = value, next
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown register argument %q", arg)
		}
		return fmt.Errorf("register does not accept positional arguments")
	}
	if (flags.bundle == "") == (flags.image == "") {
		return fmt.Errorf("exactly one of --bundle or --image is required")
	}
	return nil
}

func parseRunFlags(args []string, flags *runFlags, allowed ...runSelector) error {
	allowedSelectors := allSelectors
	if len(allowed) > 0 {
		allowedSelectors = allowed[0]
	}
	if flags.env == nil {
		flags.env = map[string]string{}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--name", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--name requires a value")
			}
			flags.name, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--image", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--image requires a value")
			}
			flags.image, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--bundle", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--bundle requires a value")
			}
			flags.bundle, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--workspace", "--ws"); matched {
			if err != nil {
				return err
			}
			if value == "" {
				flag, _, _ := strings.Cut(arg, "=")
				return fmt.Errorf("%s requires a value", flag)
			}
			flags.workspace, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--prompt", ""); matched {
			if err != nil {
				return err
			}
			flags.prompt, i = &value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--model", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--model requires a value")
			}
			flags.model, i = value, next
			continue
		}
		switch {
		case arg == "--env":
			value, next, err := consumeValue(args, i, arg)
			if err != nil {
				return err
			}
			key, envValue, err := parseEnv(value)
			if err != nil {
				return err
			}
			flags.env[key] = envValue
			i = next
		case strings.HasPrefix(arg, "--env="):
			key, envValue, err := parseEnv(strings.TrimPrefix(arg, "--env="))
			if err != nil {
				return err
			}
			flags.env[key] = envValue
		case arg == "--env-file":
			value, next, err := consumeValue(args, i, arg)
			if err != nil {
				return err
			}
			flags.envFiles = append(flags.envFiles, value)
			i = next
		case strings.HasPrefix(arg, "--env-file="):
			flags.envFiles = append(flags.envFiles, strings.TrimPrefix(arg, "--env-file="))
		case arg == "--env-auto":
			flags.envAuto = true
		case arg == "--debug":
			flags.debug = true
		case arg == "--tui":
			if flags.mode == harness.ModeACP {
				return fmt.Errorf("--tui cannot be used with --acp")
			}
			flags.mode = harness.ModeTUI
		case arg == "--acp":
			if flags.mode == harness.ModeTUI {
				return fmt.Errorf("--tui cannot be used with --acp")
			}
			flags.mode = harness.ModeACP
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown run argument %q", arg)
		default:
			return fmt.Errorf("run does not accept positional arguments")
		}
	}
	selectionCount := 0
	if flags.image != "" {
		selectionCount++
	}
	if flags.bundle != "" {
		selectionCount++
	}
	if flags.name != "" {
		selectionCount++
	}
	if selectionCount > 1 {
		return fmt.Errorf("--bundle, --image, and --name are mutually exclusive")
	}
	if selectionCount == 0 {
		return fmt.Errorf("exactly one of --bundle, --image, or --name is required")
	}
	selected := bundleSelector
	if flags.image != "" {
		selected = imageSelector
	} else if flags.name != "" {
		selected = nameSelector
	}
	if selected&allowedSelectors == 0 {
		return fmt.Errorf("run accepts only %s", selectorUsage(allowedSelectors))
	}
	if flags.prompt != nil && flags.mode != "" {
		return fmt.Errorf("--prompt cannot be used with --%s", flags.mode)
	}
	if flags.mode == harness.ModeACP && flags.workspace != "" {
		return fmt.Errorf("--workspace cannot be used with --acp; the ACP client supplies the workspace")
	}
	if flags.workspace != "" {
		workspace, err := filepath.Abs(flags.workspace)
		if err != nil {
			return err
		}
		flags.workspace = workspace
	}
	return nil
}

func selectorUsage(selectors runSelector) string {
	if selectors == bundleSelector {
		return "--bundle FILE"
	}
	if selectors == imageSelector {
		return "--image REF"
	}
	if selectors == nameSelector {
		return "--name NAME"
	}
	return "(--bundle FILE | --image REF | --name NAME)"
}

func parseRequiredName(args []string, command string) (string, error) {
	var name string
	for i := 0; i < len(args); i++ {
		value, next, matched, err := matchValueFlag(args, i, args[i], "--name", "")
		if !matched {
			if strings.HasPrefix(args[i], "-") {
				return "", fmt.Errorf("unknown %s argument %q", command, args[i])
			}
			return "", fmt.Errorf("%s does not accept positional arguments", command)
		}
		if err != nil {
			return "", err
		}
		if value == "" {
			return "", fmt.Errorf("--name requires a value")
		}
		if name != "" {
			return "", fmt.Errorf("--name may be specified only once")
		}
		name, i = value, next
	}
	if name == "" {
		return "", fmt.Errorf("--name is required")
	}
	return name, nil
}

func consumeValue(args []string, index int, flag string) (string, int, error) {
	next := index + 1
	if next >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	return args[next], next, nil
}

func parseEnv(raw string) (string, string, error) {
	key, value, ok := strings.Cut(raw, "=")
	if key == "" {
		return "", "", fmt.Errorf("--env requires KEY or KEY=VALUE")
	}
	envValue := value
	if err := (agentfile.Env{Name: key, ValueSource: agentfile.ValueSource{Value: &envValue}}).Validate("--env"); err != nil {
		return "", "", err
	}
	if ok {
		return key, value, nil
	}
	lookup, found := os.LookupEnv(key)
	if !found {
		return "", "", fmt.Errorf("environment variable %s is not set", key)
	}
	return key, lookup, nil
}
