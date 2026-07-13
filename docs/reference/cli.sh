af --help # show help & usage

af build \ # build an agent image
    --file agentfile.yaml \  # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --tag myregistry.example/myagent:latest \ # tag the created image. Default: metadata.name:metadata.version

af run # alias to af agents run

af agents list # list registered agents

af agents run \ # run agent
    myagent \ # name of registered agent
    --file agentfile.yaml \  # build agent first from given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml. Mutually exclusive with NAME and --image
    --image myregistry.example/myagent:latest \ # run an agent image directly. Mutually exclusive with NAME and --file
    --tui \ # open the harness's native interactive terminal. Mutually exclusive with --acp and --prompt
    --acp \ # serve the agent to an ACP client over stdio. Mutually exclusive with --tui, --prompt and --workspace
    --prompt "say hi" \ # replace the agent's default one-shot prompt. Mutually exclusive with --tui and --acp
    --model claude-sonnet-4-5 \ # replace the agent's default model for this run
    --workspace /path/to/dir \ # bind an existing directory to /agent/workspace. Alias: --ws
    --env KEY[=VALUE] \ # set an environment variable in the container. if VALUE is omitted, the value is taken from the current environment
    --env-file FILE \ # load environment variables from an .env file
    --env-auto \ # export runtime variables declared in the agentfile from the host environment
    --debug # stream build progress and agent stderr; failed one-shot stderr is printed even without this flag

af agents register \ # register an agent
    myagent \ # name of agent
    --file agentfile.yaml # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --image myregistry.example/myagent:latest # local image with build.agentfile labels from af build

af agents remove \ # remove a registered agent
    myagent # name of agent to remove
