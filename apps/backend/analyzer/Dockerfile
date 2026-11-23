# syntax=docker/dockerfile:1
#
# Isolated libclang analyzer for PA#3 hard rules. The image is built from
# Debian trixie pinned by digest and contains no provider key, dataset
# label or Docker socket. Go invokes it with --network none and a bounded
# stdin/stdout protocol; see README.md for the qualification record.
FROM debian@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/tmp \
    TMPDIR=/tmp

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        clang-19 \
        libclang-19-dev \
        g++-14 \
        libstdc++-14-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY analyzer.cpp generate-clang-args.sh ./
RUN sh ./generate-clang-args.sh default_clang_args.inc \
    && clang++-19 -std=c++17 -O2 -Wall -Wextra -pedantic \
        -I/usr/lib/llvm-19/include \
        -o /usr/local/bin/leetkob-analyzer analyzer.cpp \
        -L/usr/lib/llvm-19/lib -lclang -Wl,-rpath,/usr/lib/llvm-19/lib \
    && rm -f /src/analyzer.cpp /src/generate-clang-args.sh /src/default_clang_args.inc

# Unprivileged; the Go boundary also passes --user 65534:65534.
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/leetkob-analyzer"]
