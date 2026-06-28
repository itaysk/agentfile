FROM alpine:latest

RUN apk add --no-cache bash ca-certificates git nodejs npm ripgrep \
    && npm install -g --ignore-scripts @earendil-works/pi-coding-agent@latest \
    && npm cache clean --force

WORKDIR /agent/workspace
