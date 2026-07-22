af --help # show help and usage

af bundle build \ # build an agent bundle
    --file agentfile.yaml \ # source agentfile. Short: -f. Default: agentfile.yaml
    --output myagent.tar.gz # output path. Default: <name>__<version>.tar.gz

af build \ # exact convenience equivalent of af bundle build
    --file agentfile.yaml \
    --output myagent.tar.gz

af bundle run \ # run a bundle on the host; unsandboxed
    --bundle myagent.tar.gz \ # required bundle selector
    --tui \ # open the harness's native interactive terminal. Mutually exclusive with --acp and --prompt
    --acp \ # serve the bundle to an ACP client over stdio. Mutually exclusive with --tui, --prompt, and --workspace
    --prompt "say hi" \ # replace the default one-shot prompt. Mutually exclusive with --tui and --acp
    --model claude-sonnet-4-5 \ # replace the default model for this run
    --workspace /path/to/dir \ # use an existing workspace directory. Alias: --ws
    --env KEY[=VALUE] \ # set an environment variable; omit VALUE to use the current environment
    --env-file FILE \ # load environment variables from an .env file
    --env-auto \ # accept declared runtime variables from the inherited environment
    --debug # stream agent stderr; failed one-shot stderr is printed even without this flag

af image build \ # build an agent image from a bundle
    --bundle myagent.tar.gz \ # required bundle selector
    --base-image myregistry.example/agent-base:latest \ # override the harness's default base image
    --tag myregistry.example/myagent:latest # image tag. Default: <name>:<version>

af image run \ # run an image with Docker
    --image myregistry.example/myagent:latest \ # required image selector
    --tui \ # open the harness's native interactive terminal. Mutually exclusive with --acp and --prompt
    --acp \ # serve the image to an ACP client over stdio. Mutually exclusive with --tui, --prompt, and --workspace
    --prompt "say hi" \
    --model claude-sonnet-4-5 \
    --workspace /path/to/dir \
    --env KEY[=VALUE] \
    --env-file FILE \
    --env-auto \
    --debug

af agents run \ # run a registered agent
    --name myagent \ # required registered-name selector
    --tui \ # open the harness's native interactive terminal
    --acp \ # serve the registered bundle or image to an ACP client over stdio
    --prompt "say hi" \
    --model claude-sonnet-4-5 \
    --workspace /path/to/dir \
    --env KEY[=VALUE] \
    --env-file FILE \
    --env-auto \
    --debug

af run \ # convenience dispatcher; exactly one selector is required
    --bundle myagent.tar.gz | \ # equivalent to af bundle run --bundle myagent.tar.gz
    --image myregistry.example/myagent:latest | \ # equivalent to af image run --image myregistry.example/myagent:latest
    --name myagent \ # equivalent to af agents run --name myagent
    --tui \
    --acp \ # serve the selected bundle or image to an ACP client over stdio
    --prompt "say hi" \
    --model claude-sonnet-4-5 \
    --workspace /path/to/dir \
    --env KEY[=VALUE] \
    --env-file FILE \
    --env-auto \
    --debug

af ps # list local agents currently running through af

af agents register \ # register a managed bundle or local Agentfile image
    --name myagent \ # optional; inferred from bundle metadata or image labels
    --bundle myagent.tar.gz | --image myregistry.example/myagent:latest # exactly one is required

af agents list # list registered agents

af agents remove \ # remove a registered agent
    --name myagent # required registered name
