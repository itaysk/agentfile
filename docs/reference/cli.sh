af --help # show help & usage

af build \ # build an agent image (default) or portable bundle
    --target image|bundle \ # artifact type. Default: image
    --file agentfile.yaml \  # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --bundle myagent.tar.gz \ # image input that replaces --file; valid only for image builds
    --base-image myregistry.example/agent-base:latest \ # base image override; valid only for image builds. Default: selected harness image
    --tag myregistry.example/myagent:latest \ # image tag; valid only for image builds. Default: metadata.name:metadata.version
    --output myagent.tar.gz \ # bundle path; valid only for bundle builds. Default: metadata.name-metadata.version.tar.gz

af run # alias to af agents run

af agents list # list registered agents

af agents run \ # run agent
    myagent \ # name of registered agent
    --file agentfile.yaml \  # build agent first from given agentfile. Short: -f. Mutually exclusive with NAME, --bundle, and --image
    --bundle myagent.tar.gz \ # run an existing bundle with runa
    --image myregistry.example/myagent:latest \ # run an image directly with Docker
    --host \ # run a source agentfile with runa instead of Docker; unsandboxed
    --tui \ # open the harness's native interactive terminal. Mutually exclusive with --acp and --prompt
    --acp \ # serve the agent to an ACP client over stdio. Mutually exclusive with --tui, --prompt and --workspace
    --prompt "say hi" \ # replace the agent's default one-shot prompt. Mutually exclusive with --tui and --acp
    --model claude-sonnet-4-5 \ # replace the agent's default model for this run
    --workspace /path/to/dir \ # use an existing workspace directory. Alias: --ws
    --env KEY[=VALUE] \ # set an environment variable for the invocation. if VALUE is omitted, use the current environment
    --env-file FILE \ # load environment variables from an .env file
    --env-auto \ # export runtime variables declared in the agentfile from the host environment
    --debug # stream build progress and agent stderr; failed one-shot stderr is printed even without this flag

af agents register \ # register an agent
    myagent \ # name of agent
    --file agentfile.yaml # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --image myregistry.example/myagent:latest # local image with build.agentfile labels from af build

af agents remove \ # remove a registered agent
    myagent # name of agent to remove
