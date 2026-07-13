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
	buildpkg "github.com/itaysk/agentfile/internal/build"
	"github.com/itaysk/agentfile/internal/config"
	"github.com/itaysk/agentfile/internal/runner"
)

// version is set via -ldflags at release time (see .goreleaser.yaml).
var version = "dev"

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
		code, err = runRun(args[1:], stdout, stderr)
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
		fmt.Fprintln(stdout, "usage: af build [--file agentfile.yaml] [--tag TAG]")
		return nil
	}
	options := buildFlags{file: agentfile.DefaultFileName}
	if err := parseBuildFlags(args, &options); err != nil {
		return err
	}
	project, err := agentfile.Load(options.file)
	if err != nil {
		return err
	}
	tag, err := buildpkg.Build(context.Background(), buildpkg.Options{
		Project: project,
		Tag:     options.tag,
		Stdout:  stdout,
		Stderr:  stderr,
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
	if args[0] == "run" {
		return runRun(args[1:], stdout, stderr)
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

func runRun(args []string, stdout, stderr io.Writer) (int, error) {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af run [NAME | --file agentfile.yaml | --image REF] [--tui | --acp | --prompt TEXT] [--model MODEL] [--workspace DIR] [--ws DIR] [--env KEY[=VALUE]] [--env-file FILE] [--env-auto] [--debug]")
		return 0, nil
	}
	options := runFlags{file: agentfile.DefaultFileName, env: map[string]string{}}
	if err := parseRunFlags(args, &options); err != nil {
		return 1, err
	}
	runStderr := io.Discard
	if options.debug || options.mode != "" {
		runStderr = stderr
	}
	// Pull progress goes to real stderr even without --debug: a first pull can
	// take minutes and stdout stays clean either way.
	project, image, runtimeEnvNames, harness, err := loadRunSelection(options, stderr)
	if err != nil {
		return 1, err
	}
	runOptions := runner.Options{
		Project:         project,
		Image:           image,
		Harness:         harness,
		RuntimeEnvNames: runtimeEnvNames,
		Prompt:          options.prompt,
		Model:           options.model,
		Env:             options.env,
		EnvFiles:        options.envFiles,
		EnvAuto:         options.envAuto,
		Workspace:       options.workspace,
		Mode:            options.mode,
		Stdout:          stdout,
		Stderr:          runStderr,
	}
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
	entry := config.Entry{Image: options.image}
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
	registry, err := config.LoadRegistry()
	if err != nil {
		return err
	}
	registry.Put(entry)
	if err := config.SaveRegistry(registry); err != nil {
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
	registry, err := config.LoadRegistry()
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tIMAGE\tAGENTFILE")
	for _, entry := range registry.SortedEntries() {
		if entry.Image != "" {
			fmt.Fprintf(writer, "%s\t%s\t-\n", entry.Name, entry.Image)
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
	registry, err := config.LoadRegistry()
	if err != nil {
		return err
	}
	if !registry.Remove(args[0]) {
		return fmt.Errorf("agent %q is not registered", args[0])
	}
	if err := config.SaveRegistry(registry); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %s\n", args[0])
	return nil
}

func loadRunSelection(options runFlags, stderr io.Writer) (*agentfile.Project, string, []string, string, error) {
	if options.image != "" {
		image, runtimeEnvNames, harness, err := loadRunImage(options.image, stderr)
		return nil, image, runtimeEnvNames, harness, err
	}
	if options.fileSet {
		project, err := agentfile.Load(options.file)
		return project, "", nil, "", err
	}
	if options.name != "" {
		registry, err := config.LoadRegistry()
		if err != nil {
			return nil, "", nil, "", err
		}
		entry, ok := registry.Agents[options.name]
		if !ok {
			return nil, "", nil, "", fmt.Errorf("agent %q is not registered", options.name)
		}
		if entry.Image != "" {
			image, runtimeEnvNames, harness, err := loadRunImage(entry.Image, stderr)
			return nil, image, runtimeEnvNames, harness, err
		}
		project, err := agentfile.Load(entry.AgentfilePath)
		return project, "", nil, "", err
	}
	project, err := agentfile.Load(agentfile.DefaultFileName)
	return project, "", nil, "", err
}

func loadRunImage(image string, stderr io.Writer) (string, []string, string, error) {
	ctx := context.Background()
	info, err := runner.ReadImageInfo(ctx, "", image)
	if err != nil {
		if err := runner.PullImage(ctx, "", image, stderr); err != nil {
			return "", nil, "", err
		}
		if info, err = runner.ReadImageInfo(ctx, "", image); err != nil {
			return "", nil, "", err
		}
	}
	return image, info.RuntimeEnvNames, info.Harness, nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af COMMAND [ARGS]

Commands:
  build              build an agent image
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
	file string
	tag  string
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
	env       map[string]string
	envFiles  []string
	envAuto   bool
	workspace string
	prompt    *string
	model     string
	debug     bool
	mode      runner.RunMode
}

// matchStrFlag recognizes "--long value", "short value", "--long=value", and "short=value".
// matched is false when arg is not this flag; err is non-nil only when a value
// is required but missing.
func matchStrFlag(args []string, i int, arg, long, short string) (value string, next int, matched bool, err error) {
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

func parseBuildFlags(args []string, options *buildFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchStrFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			options.file, i = value, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--tag", ""); matched {
			if err != nil {
				return err
			}
			options.tag, i = value, next
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown build argument %q", arg)
		}
		return fmt.Errorf("build does not accept positional arguments")
	}
	return nil
}

func parseRegisterFlags(args []string, options *registerFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchStrFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			options.file, options.fileSet, i = value, true, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--image", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				flag, _, _ := strings.Cut(arg, "=")
				return fmt.Errorf("%s requires a value", flag)
			}
			options.image, i = value, next
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown register argument %q", arg)
		}
		if options.name != "" {
			return fmt.Errorf("register accepts at most one NAME")
		}
		options.name = arg
	}
	if options.fileSet && options.image != "" {
		return fmt.Errorf("--file and --image cannot be used together")
	}
	return nil
}

func parseRunFlags(args []string, options *runFlags) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchStrFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			options.file, options.fileSet, i = value, true, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--image", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--image requires a value")
			}
			options.image, i = value, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--workspace", "--ws"); matched {
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
			options.workspace, i = abs, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--prompt", ""); matched {
			if err != nil {
				return err
			}
			options.prompt, i = &value, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--model", ""); matched {
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("--model requires a value")
			}
			options.model, i = value, next
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
			options.env[key] = envValue
			i = next
		case strings.HasPrefix(arg, "--env="):
			key, envValue, err := parseEnv(strings.TrimPrefix(arg, "--env="))
			if err != nil {
				return err
			}
			options.env[key] = envValue
		case arg == "--env-file":
			value, next, err := consumeValue(args, i, arg)
			if err != nil {
				return err
			}
			options.envFiles = append(options.envFiles, value)
			i = next
		case strings.HasPrefix(arg, "--env-file="):
			options.envFiles = append(options.envFiles, strings.TrimPrefix(arg, "--env-file="))
		case arg == "--env-auto":
			options.envAuto = true
		case arg == "--debug":
			options.debug = true
		case arg == "--tui":
			if options.mode == runner.RunModeACP {
				return fmt.Errorf("--tui cannot be used with --acp")
			}
			options.mode = runner.RunModeTUI
		case arg == "--acp":
			if options.mode == runner.RunModeTUI {
				return fmt.Errorf("--tui cannot be used with --acp")
			}
			options.mode = runner.RunModeACP
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown run argument %q", arg)
		default:
			if options.name != "" {
				return fmt.Errorf("run accepts at most one NAME")
			}
			options.name = arg
		}
	}
	if options.image != "" && options.fileSet {
		return fmt.Errorf("--file and --image cannot be used together")
	}
	if options.image != "" && options.name != "" {
		return fmt.Errorf("NAME and --image cannot be used together")
	}
	if options.fileSet && options.name != "" {
		return fmt.Errorf("NAME and --file cannot be used together")
	}
	if options.prompt != nil && options.mode != "" {
		return fmt.Errorf("--prompt cannot be used with --%s", options.mode)
	}
	if options.mode == runner.RunModeACP && options.workspace != "" {
		return fmt.Errorf("--workspace cannot be used with --acp; the ACP client supplies the workspace")
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
