# syntax=docker/dockerfile:1

# no default: .go-version is the single source of truth and make/CI pass it in, so the builder can
# never lag go.mod's go directive (the official go images pin GOTOOLCHAIN=local, so an older
# builder fails the build rather than fetching a newer toolchain). build by hand with:
#   docker build --build-arg GO_VERSION=$(cat .go-version) .
ARG GO_VERSION

# the build stage runs on the builder's own architecture and cross-compiles for the target, so the
# multi-arch image builds in seconds rather than minutes under qemu emulation
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=docker

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -mod=vendor \
    -ldflags "-s -w -X github.com/katbyte/go-kt/version.Version=${VERSION} -X github.com/katbyte/go-kt/version.GitCommit=${GIT_COMMIT}" \
    -o /out/taproot ./cmd/taproot

# tzdata makes TZ work, for the times in the log and in the names of backups. taproot only ever
# talks plain http to controllers on the local network, so there are no certificates to carry.
# upgrade first so a release rebuild picks up alpine security fixes the base image tag has not
# been rebuilt with yet
FROM alpine:3.24
RUN apk upgrade --no-cache && apk add --no-cache tzdata

# the volume: the controllers file, which holds the tokens, and the backups
ENV TAPROOT_CONFIG_DIR=/config
VOLUME /config
COPY --from=build /out/taproot /usr/bin/taproot

EXPOSE 7668
CMD ["taproot", "serve", "7668"]
