FROM alpine:latest

RUN apk add --no-cache bash ca-certificates git libgcc libstdc++ ripgrep wget \
    && wget -O /etc/apk/keys/claude-code.rsa.pub https://downloads.claude.ai/keys/claude-code.rsa.pub \
    && echo "https://downloads.claude.ai/claude-code/apk/latest" >> /etc/apk/repositories \
    && apk add --no-cache claude-code

ENV USE_BUILTIN_RIPGREP=0

WORKDIR /agent/workspace
