af --help # show help & usage

af build \ # build an agent image
    --file Agentfile.yaml \  # use given Agentfile. Short: -f. Default: Agentfile.yaml
    --project /path/to/project/dir \ # use given project dir. Default: current working directory
    --tag https://myregistry:1234/myagent:latest \ # tag the created image. Default: Agentfile/metadata.name:Agentfile/metadata.version

af run # alias to af agents run

af agents list # list registered agents

af agents register \ # register and agent
    myagent \ # name of agent
    --file Agentfile.yaml \  # use given Agentfile. Short: -f. Default: Agentfile.yaml
    --project /path/to/project/dir # use given project dir. Default: current working directory

af agents run \ # run agent
    myagent \ # name of registered agent
    --file Agentfile.yaml \  # build agent first from given Agentfile. Short: -f. Default: Agentfile.yaml
    --project /path/to/project/dir \ # build agent first from given project dir. Default: current working directory
    --in /path/to/dir \ # alias to --workspace.hostBindPath
    --field value \ # set a spec-level Agentfile field to the given string value
    --parent.field value # set a spec-level Agentfile nested field to the given string value
