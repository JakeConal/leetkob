# Fixed C++17 execution image for the PA#3 Linux sandbox.
#
# The trusted controller builds this image and creates every student
# container from it. The image has no Docker client, no host files, and no
# credentials. Compilation and execution both happen in containers created
# from this image; Docker control stays outside.
#
# Base pin: debian:bookworm-slim multi-arch index observed 2026-11-23.
# BuildKit selects the image for the builder's architecture from this index.

FROM debian:bookworm-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends g++ \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 sandbox \
    && useradd --uid 10001 --gid 10001 --home-dir /work --no-create-home --shell /usr/sbin/nologin sandbox \
    && mkdir -p /work \
    && chown 10001:10001 /work \
    && chmod 0755 /work

USER 10001:10001
WORKDIR /work
