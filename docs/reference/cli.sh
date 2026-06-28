af --help # show help & usage

af build \ # build an agent image
    --file agentfile.yaml \  # use given agentfile. Short: -f. Default: agentfile.yaml
    --project /path/to/project/dir \ # use given project dir. Default: current working directory
    --tag https://myregistry:1234/myagent:latest \ # tag the created image. Default: agentfile/metadata.name:agentfile/metadata.version

af run # alias to af agents run

af agents list # list registered agents

af agents run \ # run agent
    myagent \ # name of registered agent
    --file agentfile.yaml \  # build agent first from given agentfile. Short: -f. Default: agentfile.yaml
    --project /path/to/project/dir \ # build agent first from given project dir. Default: current working directory
    --in /path/to/dir \ # alias to --workspace.hostBindPath
    --parent.field value # set a spec-level agentfile nested field to the given string value
    --env KEY[=VALUE] \ # set an environment variable in the container. if VALUE is omitted, the value is taken from the current environment
    --env-file FILE # load environment variables from an .env file

af agents register \ # register and agent
    myagent \ # name of agent
    --file agentfile.yaml \  # use given agentfile. Short: -f. Default: agentfile.yaml
    --project /path/to/project/dir # use given project dir. Default: current working directory

af agents remove \ # remove a registered agent
    myagent # name of agent to remove
