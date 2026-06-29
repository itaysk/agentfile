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

	var err error
	switch args[0] {
	case "build":
		err = runBuild(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "agents":
		return runAgents(args[1:], stdout, stderr)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	return 0
}

func runBuild(args []string, stdout, stderr io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af build [--file agentfile.yaml] [--project DIR] [--tag TAG]")
		return nil
	}
	options := buildFlags{file: agentfile.DefaultFileName}
	if err := parseBuildFlags(args, &options); err != nil {
		return err
	}
	project, err := agentfile.Load(options.project, options.file)
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

func runAgents(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printAgentsHelp(stdout)
		return 0
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
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	return 0
}

func runRun(args []string, stdout, stderr io.Writer) int {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af run [NAME] [--file agentfile.yaml] [--project DIR] [--in DIR] [--here] [--env KEY[=VALUE]] [--env-file FILE] [field overrides]")
		return 0
	}
	options := runFlags{file: agentfile.DefaultFileName, env: map[string]string{}}
	if err := parseRunFlags(args, &options); err != nil {
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	project, tag, err := loadRunSelection(options)
	if err != nil {
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	if err := applyMutations(project, options.mutations); err != nil {
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	exitCode, err := runner.Run(context.Background(), runner.Options{
		Project:  project,
		Tag:      tag,
		Env:      options.env,
		EnvFiles: options.envFiles,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if err != nil {
		fmt.Fprintln(stderr, "af:", err)
		return 1
	}
	return exitCode
}

func runRegister(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		fmt.Fprintln(stdout, "usage: af agents register [NAME] [--file agentfile.yaml] [--project DIR]")
		return nil
	}
	options := registerFlags{file: agentfile.DefaultFileName}
	if err := parseRegisterFlags(args, &options); err != nil {
		return err
	}
	project, err := agentfile.Load(options.project, options.file)
	if err != nil {
		return err
	}
	name := options.name
	if name == "" {
		name = project.AgentFile.Metadata.Name
	}
	registry, err := config.LoadRegistry()
	if err != nil {
		return err
	}
	registry.Put(config.Entry{
		Name:            name,
		ProjectDir:      project.ProjectDir,
		AgentfilePath:   project.AgentfilePath,
		DefaultImageTag: project.DefaultImageTag(),
	})
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
	fmt.Fprintln(writer, "NAME\tIMAGE\tPROJECT\tAGENTFILE")
	for _, entry := range registry.SortedEntries() {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", entry.Name, entry.DefaultImageTag, entry.ProjectDir, entry.AgentfilePath)
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

func loadRunSelection(options runFlags) (*agentfile.Project, string, error) {
	if options.fileSet || options.projectSet {
		project, err := agentfile.Load(options.project, options.file)
		return project, "", err
	}
	if options.name != "" {
		registry, err := config.LoadRegistry()
		if err != nil {
			return nil, "", err
		}
		entry, ok := registry.Agents[options.name]
		if !ok {
			return nil, "", fmt.Errorf("agent %q is not registered", options.name)
		}
		project, err := agentfile.Load(entry.ProjectDir, entry.AgentfilePath)
		return project, entry.DefaultImageTag, err
	}
	project, err := agentfile.Load("", agentfile.DefaultFileName)
	return project, "", err
}

func applyMutations(project *agentfile.Project, mutations []fieldMutation) error {
	for _, mutation := range mutations {
		if err := project.ApplyOverride(mutation.path, mutation.value); err != nil {
			return err
		}
	}
	return nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af COMMAND [ARGS]

Commands:
  build              build an agent image
  run                run an agent (alias for af agents run)
  agents list        list registered agents
  agents run         run an agent
  agents register    register an agent
  agents remove      remove a registered agent
  version            print the af version

Use "af agents --help" for agent registry commands.`)
}

func printAgentsHelp(w io.Writer) {
	fmt.Fprintln(w, `usage: af agents COMMAND [ARGS]

Commands:
  run [NAME]         run a registered or local agent
  register [NAME]    register an agent
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
	file    string
	project string
	tag     string
}

type registerFlags struct {
	file    string
	project string
	name    string
}

type runFlags struct {
	name       string
	file       string
	project    string
	fileSet    bool
	projectSet bool
	env        map[string]string
	envFiles   []string
	mutations  []fieldMutation
}

type fieldMutation struct {
	path  string
	value string
}

// matchStrFlag recognizes "--long value", "short value", and "--long=value".
// matched is false when arg is not this flag; err is non-nil only when a value
// is required but missing.
func matchStrFlag(args []string, i int, arg, long, short string) (value string, next int, matched bool, err error) {
	switch {
	case arg == long || (short != "" && arg == short):
		v, n, e := consumeValue(args, i, arg)
		return v, n, true, e
	case strings.HasPrefix(arg, long+"="):
		return strings.TrimPrefix(arg, long+"="), i, true, nil
	}
	return "", i, false, nil
}

// matchAssetFlag handles --<asset> / --<asset>=value for each spec asset field,
// recording an override mutation. matched is false when arg is not an asset flag.
func matchAssetFlag(args []string, i int, arg string, options *runFlags) (matched bool, next int, err error) {
	for _, asset := range agentfile.AssetFields() {
		if value, n, ok, err := matchStrFlag(args, i, arg, "--"+asset, ""); ok {
			if err != nil {
				return true, i, err
			}
			options.mutations = append(options.mutations, fieldMutation{path: asset, value: value})
			return true, n, nil
		}
	}
	return false, i, nil
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
		if value, next, matched, err := matchStrFlag(args, i, arg, "--project", ""); matched {
			if err != nil {
				return err
			}
			options.project, i = value, next
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
			options.file, i = value, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--project", ""); matched {
			if err != nil {
				return err
			}
			options.project, i = value, next
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
	return nil
}

func parseRunFlags(args []string, options *runFlags) error {
	var inSet, hereSet bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, next, matched, err := matchStrFlag(args, i, arg, "--file", "-f"); matched {
			if err != nil {
				return err
			}
			options.file, options.fileSet, i = value, true, next
			continue
		}
		if value, next, matched, err := matchStrFlag(args, i, arg, "--project", ""); matched {
			if err != nil {
				return err
			}
			options.project, options.projectSet, i = value, true, next
			continue
		}
		// Asset flags (--prompt, --systemPrompt) come from the spec's asset
		// list; the override layer turns a bare value into a text source.
		if matched, next, err := matchAssetFlag(args, i, arg, options); matched {
			if err != nil {
				return err
			}
			i = next
			continue
		}
		switch {
		case arg == "--in":
			value, next, err := consumeValue(args, i, arg)
			if err != nil {
				return err
			}
			if hereSet {
				return fmt.Errorf("--in and --here cannot be used together")
			}
			abs, err := filepath.Abs(value)
			if err != nil {
				return err
			}
			inSet = true
			options.mutations = append(options.mutations, fieldMutation{path: "workspace.hostBindPath", value: abs})
			i = next
		case strings.HasPrefix(arg, "--in="):
			if hereSet {
				return fmt.Errorf("--in and --here cannot be used together")
			}
			abs, err := filepath.Abs(strings.TrimPrefix(arg, "--in="))
			if err != nil {
				return err
			}
			inSet = true
			options.mutations = append(options.mutations, fieldMutation{path: "workspace.hostBindPath", value: abs})
		case arg == "--here":
			if inSet {
				return fmt.Errorf("--in and --here cannot be used together")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			hereSet = true
			options.mutations = append(options.mutations, fieldMutation{path: "workspace.hostBindPath", value: cwd})
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
		case strings.HasPrefix(arg, "--") && strings.Contains(arg, "."):
			path, value, next, err := parseOverrideArg(args, i)
			if err != nil {
				return err
			}
			options.mutations = append(options.mutations, fieldMutation{path: path, value: value})
			i = next
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown run argument %q", arg)
		default:
			if options.name != "" {
				return fmt.Errorf("run accepts at most one NAME")
			}
			options.name = arg
		}
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

func parseOverrideArg(args []string, index int) (string, string, int, error) {
	arg := strings.TrimPrefix(args[index], "--")
	if before, after, ok := strings.Cut(arg, "="); ok {
		return before, after, index, nil
	}
	value, next, err := consumeValue(args, index, "--"+arg)
	if err != nil {
		return "", "", index, err
	}
	return arg, value, next, nil
}

func parseEnv(raw string) (string, string, error) {
	key, value, ok := strings.Cut(raw, "=")
	if key == "" {
		return "", "", fmt.Errorf("--env requires KEY or KEY=VALUE")
	}
	envValue := value
	if err := (agentfile.Env{Name: key, Value: &envValue}).Validate("--env"); err != nil {
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
