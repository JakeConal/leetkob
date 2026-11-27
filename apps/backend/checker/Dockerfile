# PA#3 dataset-independent checker image.
#
# Build context: apps/backend
#   docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
#
# Both base images are pinned by resolved digest (2026-11-24). The runtime
# image carries clang/libclang only because the analyzer parses C++17 with
# system include paths; the Go binary is static and the analyzer runs as a
# child process. No provider keys, no Docker socket and no shell-out to the
# network are involved.
FROM golang@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build

WORKDIR /src
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
COPY cmd/checker ./cmd/checker
COPY cmd/analyze-isolated ./cmd/analyze-isolated
COPY internal ./internal

RUN go build -o /checker ./cmd/checker \
    && go build -o /analyze-isolated ./cmd/analyze-isolated

FROM debian@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        clang \
        libclang-dev \
        libstdc++-14-dev \
    && rm -rf /var/lib/apt/lists/*

COPY native/cpp-analyzer/analyzer.c /tmp/analyzer.c
RUN LIBDIR="$(dirname "$(clang -print-file-name=libclang.so)")" \
    && clang -std=c11 -D_XOPEN_SOURCE=700 /tmp/analyzer.c -o /usr/local/bin/cpp-analyzer \
        -L"$LIBDIR" -lclang -Wl,-rpath,"$LIBDIR" \
    && rm /tmp/analyzer.c

COPY --from=build /checker /checker
COPY --from=build /analyze-isolated /analyze-isolated

ENV CHECKER_ANALYZER=/usr/local/bin/cpp-analyzer

USER 65534:65534
ENTRYPOINT ["/checker"]
