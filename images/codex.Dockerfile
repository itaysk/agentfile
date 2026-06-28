FROM alpine:latest

RUN apk add --no-cache bash ca-certificates curl git libgcc libstdc++ ripgrep \
    && curl -fsSL https://chatgpt.com/codex/install.sh | CODEX_NON_INTERACTIVE=1 sh

ENV PATH="/root/.local/bin:${PATH}"

WORKDIR /agent/workspace
