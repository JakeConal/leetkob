# PA#3 dataset-independent checker image (task 7.1).
#
# Build context: apps/backend
#   docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:7.1 apps/backend
#
# The Go base is pinned by resolved digest (2026-11-23). Re-resolve with
#   docker buildx imagetools inspect golang:1.27.4-bookworm
# before changing the digest. The runtime stage is scratch: a static binary
# with no shell, no package manager and no network tooling.
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
COPY cmd/checker ./cmd/checker
COPY internal/checker ./internal/checker

# No `go mod download`: cmd/checker imports only the standard library and
# internal/checker, so a clean build needs no module proxy access.
RUN --network=none go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/checker ./cmd/checker

FROM scratch

COPY --from=build /out/checker /checker

# Numeric non-root identity; scratch has no /etc/passwd and needs none.
USER 65532:65532

ENTRYPOINT ["/checker"]
