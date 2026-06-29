af --help # show help & usage

af build \ # build an agent image
    --file agentfile.yaml \  # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --tag myregistry.example/myagent:latest \ # tag the created image. Default: metadata.name:metadata.version

af run # alias to af agents run

af agents list # list registered agents

af agents run \ # run agent
    myagent \ # name of registered agent
    --file agentfile.yaml \  # build agent first from given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml
    --workspace /path/to/dir \ # bind an existing directory to /agent/workspace. Alias: --ws
    --prompt "say hi" \ # replace spec.prompt with an inline text source for this run
    --parent.field value # set a spec-level agentfile nested field to the given string value
    --env KEY[=VALUE] \ # set an environment variable in the container. if VALUE is omitted, the value is taken from the current environment
    --env-file FILE # load environment variables from an .env file

af agents register \ # register and agent
    myagent \ # name of agent
    --file agentfile.yaml # use given agentfile. Short: -f. Relative to current directory or absolute. Default: agentfile.yaml

af agents remove \ # remove a registered agent
    myagent # name of agent to remove
