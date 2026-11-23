# syntax=docker/dockerfile:1
#
# Fixed LeeTKOB C++17 execution image (PA#3 task 3.1).
#
# The base image is pinned to a resolved digest. Debian 13 (trixie) ships
# g++ 14, which fully supports -std=c++17. The image contains only the
# compiler, its runtime dependencies and the small run harness; there is
# no shell tooling beyond what Debian slim provides, no package manager
# indexes, no network configuration and no secrets.
#
# Build (from apps/backend):
#   docker build --tag leetkob-sandbox:pa3 sandbox
#
# The controller always supplies the isolation flags (network none,
# non-root user, read-only root, dropped capabilities, no privileged
# mode, no host binds beyond the copied work directory). USER here is
# defence in depth: even a caller that omitted --user would not run as
# root.

FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

ARG DEBIAN_FRONTEND=noninteractive

# Pinned Debian package version: package repositories keep the current
# stable version available, so the build fails rather than silently
# drifting if the pin is no longer resolvable. The resolved versions of
# every installed package are recorded in README.md after a clean build.
ARG GCC_VERSION=4:14.2.0-1

RUN apt-get update \
 && apt-get install --yes --no-install-recommends \
      g++="${GCC_VERSION}" \
      libstdc++6 \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

# Small fixed run harness. The controller mounts the disposable work
# directory read-only for execution, so the binary is executed in place.
COPY run-harness /usr/local/bin/run-harness
RUN chmod 0755 /usr/local/bin/run-harness

# Unprivileged runtime identity; the controller also passes --user
# 65534:65534 explicitly.
USER 65534:65534

WORKDIR /work

LABEL org.opencontainers.image.title="leetkob-sandbox" \
      org.opencontainers.image.description="Fixed Linux C++17 compile/run boundary for LeeTKOB PA#3" \
      org.opencontainers.image.source="https://github.com/JakeConal/leetkob" \
      org.opencontainers.image.base.name="docker.io/library/debian:trixie-slim" \
      org.opencontainers.image.base.digest="sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2" \
      org.leetkob.toolchain.gxx.package="4:14.2.0-1" \
      org.leetkob.toolchain.gxx.version="14.2.0-19"

# No ENTRYPOINT: the controller supplies either the fixed g++ invocation or
# the run harness path, so the exact executed command is explicit and
# inspectable. The image also contains no services, libc proxy or network
# configuration.
