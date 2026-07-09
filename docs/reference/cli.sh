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
    --workspace /path/to/dir \ # bind an existing directory to /agent/workspace. Alias: --ws
    --prompt "say hi" \ # replace spec.prompt with an inline text source for this run
    --parent.field value # set a spec-level agentfile nested field to the given string value
    --env KEY[=VALUE] \ # set an environment variable in the container. if VALUE is omitted, the value is taken from the current environment
    --env-file FILE \ # load environment variables from an .env file
    --debug # print build progress and agent stderr

af agents register \ # register an agent
    myagent \ # name of agent
    --file agentfile.yaml # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --image myregistry.example/myagent:latest # local image with build.agentfile labels from af build

af agents remove \ # remove a registered agent
    myagent # name of agent to remove
