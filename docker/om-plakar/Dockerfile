# syntax=docker/dockerfile:1

# The plakar lab: a single node om3, with the om binary built from this tree
# and the plakar binary of the reference release.

FROM golang:alpine3.24@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder

ARG PLAKAR_VERSION=v1.1.7

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" github.com/PlakarKorp/plakar@${PLAKAR_VERSION}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /opt/om ./cmd/om/

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# ssh and ssh-agent for the sftp stores, coreutils and diffutils for the
# restore checks
RUN apk add --no-cache openssh-client coreutils diffutils

COPY --from=builder /opt/om /usr/bin/om
COPY --from=builder /go/bin/plakar /usr/bin/plakar
COPY docker/om-plakar/lab /lab

ENTRYPOINT ["/usr/bin/om"]
CMD ["daemon", "run"]
