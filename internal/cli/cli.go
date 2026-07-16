package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

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
		err = runBuild(args[1:], stdout, stderr)
	case "run":
		code, err = runAgent(args[1:], stdout, stderr)
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

func runBuild(args []string, stdout, stderr io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af build [--target image --file agentfile.yaml --base-image REF --tag TAG | --target image --bundle FILE --base-image REF --tag TAG | --target bundle --file agentfile.yaml --output FILE]")
		return nil
	}
	options := buildFlags{file: agentfile.DefaultFileName, target: "image"}
	if err := parseBuildFlags(args, &options); err != nil {
		return err
	}
	if options.target == "bundle" {
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
	buildOptions := runner.BuildImageOptions{BundlePath: options.bundle, BaseImage: options.baseImage, Tag: options.tag, Stdout: stdout, Stderr: stderr}
	if options.bundle == "" {
		project, err := agentfile.Load(options.file)
		if err != nil {
			return err
		}
		buildOptions.Project = project
	}
	tag, err := runner.BuildImage(context.Background(), buildOptions)
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
	if args[0] == "run" {
		return runAgent(args[1:], stdout, stderr)
	}
	var err error
	switch args[0] {
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

func runAgent(args []string, stdout, stderr io.Writer) (int, error) {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af run [NAME | --file agentfile.yaml | --bundle FILE | --image REF] [--host] [--tui | --acp | --prompt TEXT] [--model MODEL] [--workspace DIR] [--ws DIR] [--env KEY[=VALUE]] [--env-file FILE] [--env-auto] [--debug]")
		return 0, nil
	}
	options := runFlags{file: agentfile.DefaultFileName, env: map[string]string{}}
	if err := parseRunFlags(args, &options); err != nil {
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
	if options.host && runOptions.Project != nil {
		tempDir, err := os.MkdirTemp("", "agentfile-run-bundle-*")
		if err != nil {
			return 1, err
		}
		defer os.RemoveAll(tempDir)
		runOptions.BundlePath = filepath.Join(tempDir, "agent.tar.gz")
		if err := bundle.Build(runOptions.Project, runOptions.BundlePath); err != nil {
			return 1, err
		}
		runOptions.Project = nil
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
	return runner.Run(context.Background(), runOptions)
}

func runRegister(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af agents register [NAME] [--file agentfile.yaml | --image REF]")
		return nil
	}
	options := registerFlags{file: agentfile.DefaultFileName}
	if err := parseRegisterFlags(args, &options); err != nil {
		return err
	}
	name := options.name
	entry := registry.Entry{ImageRef: options.image}
	if options.image != "" {
		info, err := runner.ReadImageInfo(context.Background(), "", options.image)
		if err != nil {
			return fmt.Errorf("%w (docker pull the image first if it is not local)", err)
		}
		if name == "" {
			name = info.Metadata.Name
		}
	} else {
		project, err := agentfile.Load(options.file)
		if err != nil {
			return err
		}
		if name == "" {
			name = project.AgentFile.Metadata.Name
		}
		entry.AgentfilePath = project.AgentfilePath
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
	fmt.Fprintln(writer, "NAME\tIMAGE\tAGENTFILE")
	for _, entry := range reg.SortedEntries() {
		if entry.ImageRef != "" {
			fmt.Fprintf(writer, "%s\t%s\t-\n", entry.Name, entry.ImageRef)
			continue
		}
		project, err := agentfile.Load(entry.AgentfilePath)
		if err != nil {
			return err
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", entry.Name, project.DefaultImageTag(), entry.AgentfilePath)
	}
	return writer.Flush()
}

func runRemove(args []string, stdout io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "usage: af agents remove NAME")
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: af agents remove NAME")
	}
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	if !reg.Remove(args[0]) {
		return fmt.Errorf("agent %q is not registered", args[0])
	}
	if err := registry.Save(reg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %s\n", args[0])
	return nil
}

func selectRunInput(flags runFlags, progress io.Writer) (runner.Options, error) {
	if flags.bundle != "" {
		return runner.Options{BundlePath: flags.bundle}, nil
	}
	if flags.image != "" {
		if flags.host {
			return runner.Options{}, fmt.Errorf("--image cannot be used with --host")
		}
		info, err := readOrPullImageInfo(flags.image, progress)
		if err != nil {
			return runner.Options{}, err
		}
		return runner.Options{ImageRef: flags.image, RuntimeEnvNames: info.RuntimeEnvNames, HarnessName: info.HarnessName}, nil
	}
	if flags.fileSet {
		project, err := agentfile.Load(flags.file)
		return runner.Options{Project: project}, err
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
		if entry.ImageRef != "" {
			if flags.host {
				return runner.Options{}, fmt.Errorf("registered image %q cannot be used with --host", flags.name)
			}
			info, err := readOrPullImageInfo(entry.ImageRef, progress)
			if err != nil {
				return runner.Options{}, err
			}
			return runner.Options{ImageRef: entry.ImageRef, RuntimeEnvNames: info.RuntimeEnvNames, HarnessName: info.HarnessName}, nil
		}
		project, err := agentfile.Load(entry.AgentfilePath)
		return runner.Options{Project: project}, err
	}
	project, err := agentfile.Load(agentfile.DefaultFileName)
	return runner.Options{Project: project}, err
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
  build              build an agent bundle or image
  run                run an agent (alias for af agents run)
  agents list        list registered agents
  agents run         run an agent
  agents register    register an agent or image
  agents remove      remove a registered agent
  version            print the af version

Use "af agents --help" for agent registry commands.`)
}

func printAgentsHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af agents COMMAND [ARGS]

Commands:
  run [NAME]         run a registered or local agent
  register [NAME]    register an agent or image
  list               list registered agents
  remove NAME        remove a registered agent`)
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

type buildFlags struct {
	file      string
	fileSet   bool
	bundle    string
	target    string
	tag       string
	baseImage string
	output    string
}

type registerFlags struct {
	file    string
	fileSet bool
	image   string
	name    string
}

type runFlags struct {
	name      string
	file      string
	fileSet   bool
	image     string
	bundle    string
	host      bool
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

func parseBuildFlags(args []string, flags *buildFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			flags.file, flags.fileSet, i = value, true, next
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
		if value, next, matched, err := matchValueFlag(args, i, arg, "--target", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--target requires a value")
			}
			flags.target, i = value, next
			continue
		}
		if value, next, matched, err := matchValueFlag(args, i, arg, "--tag", ""); matched {
			if err != nil {
				return err
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
			return fmt.Errorf("unknown build argument %q", arg)
		}
		return fmt.Errorf("build does not accept positional arguments")
	}
	if flags.target == "" {
		flags.target = "image"
	}
	if flags.target != "image" && flags.target != "bundle" {
		return fmt.Errorf("unsupported build target %q", flags.target)
	}
	if flags.fileSet && flags.bundle != "" {
		return fmt.Errorf("--file and --bundle cannot be used together")
	}
	if flags.target == "bundle" {
		if flags.bundle != "" {
			return fmt.Errorf("--bundle is valid only for image builds")
		}
		if flags.tag != "" {
			return fmt.Errorf("--tag is valid only for image builds")
		}
		if flags.baseImage != "" {
			return fmt.Errorf("--base-image is valid only for image builds")
		}
	} else if flags.output != "" {
		return fmt.Errorf("--output is valid only for bundle builds")
	}
	return nil
}

func parseRegisterFlags(args []string, flags *registerFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			flags.file, flags.fileSet, i = value, true, next
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
		if flags.name != "" {
			return fmt.Errorf("register accepts at most one NAME")
		}
		flags.name = arg
	}
	if flags.fileSet && flags.image != "" {
		return fmt.Errorf("--file and --image cannot be used together")
	}
	return nil
}

func parseRunFlags(args []string, flags *runFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchValueFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			flags.file, flags.fileSet, i = value, true, next
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
			abs, err := filepath.Abs(value)
			if err != nil {
				return err
			}
			flags.workspace, i = abs, next
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
		case arg == "--host":
			flags.host = true
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
			if flags.name != "" {
				return fmt.Errorf("run accepts at most one NAME")
			}
			flags.name = arg
		}
	}
	selectionCount := 0
	if flags.image != "" {
		selectionCount++
	}
	if flags.bundle != "" {
		selectionCount++
	}
	if flags.fileSet {
		selectionCount++
	}
	if flags.name != "" {
		selectionCount++
	}
	if selectionCount > 1 {
		return fmt.Errorf("NAME, --file, --bundle, and --image are mutually exclusive")
	}
	if flags.image != "" && flags.host {
		return fmt.Errorf("--image cannot be used with --host")
	}
	if flags.prompt != nil && flags.mode != "" {
		return fmt.Errorf("--prompt cannot be used with --%s", flags.mode)
	}
	if flags.mode == harness.ModeACP && flags.workspace != "" {
		return fmt.Errorf("--workspace cannot be used with --acp; the ACP client supplies the workspace")
	}
	if flags.mode == harness.ModeACP && (flags.host || flags.bundle != "") {
		return fmt.Errorf("--acp is not supported with host execution")
	}
	return nil
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
