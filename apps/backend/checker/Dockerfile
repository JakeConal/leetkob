# PA#3 dataset-independent checker image (tasks 7.1-7.2).
#
# The image embeds the qualified libclang analyzer as a bundled executable,
# so MODEL_OFF=1 decides ban-list constraints by shelling out to a local
# child process. It needs no Docker socket, no nested container, no network
# and no provider key.
#
# Build context: apps/backend. The analyzer image is a prerequisite because
# the final stage reuses its pinned Debian + Clang runtime and binary:
#   docker build -f apps/backend/analyzer/Dockerfile -t leetkob-analyzer:pa3 apps/backend/analyzer
#   docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
#
# The Go base is pinned by resolved digest (2026-11-23). Re-resolve with
#   docker buildx imagetools inspect golang:1.27.4-bookworm
# before changing the digest.
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
COPY cmd/checker ./cmd/checker
COPY internal/analyzer ./internal/analyzer
COPY internal/budget ./internal/budget
COPY internal/checker ./internal/checker
COPY internal/model ./internal/model
COPY internal/sanitize ./internal/sanitize

# The build needs no module proxy: the checker imports only the standard
# library and these internal packages.
RUN --network=none go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/checker ./cmd/checker

# Runtime = the qualified analyzer image. It supplies /usr/local/bin/
# leetkob-analyzer plus the exact libclang/libstdc++ runtime proven by the
# analyzer trap suite. No Docker CLI or socket is present.
FROM leetkob-analyzer:pa3

COPY --from=build /out/checker /checker

ENV CHECKER_ANALYZER=/usr/local/bin/leetkob-analyzer \
    HOME=/tmp \
    TMPDIR=/tmp

# Numeric non-root identity; the analyzer image already runs as 65534.
USER 65534:65534

ENTRYPOINT ["/checker"]
