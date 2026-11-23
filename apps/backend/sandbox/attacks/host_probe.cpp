// Attack: host mount and Docker socket probe. Expected containment: the
// socket is absent, the canary host file is absent, and the only writable
// bind is the disposable /work copy.
#include <fcntl.h>
#include <unistd.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>

#ifndef LEETKOB_CANARY_PATH
#define LEETKOB_CANARY_PATH "/nonexistent/leetkob-canary"
#endif

namespace {
void probeFile(const char* label, const char* path) {
    int fd = open(path, O_RDONLY);
    std::printf("%s=%s\n", label, fd >= 0 ? "visible" : "absent");
    if (fd >= 0) {
        close(fd);
    }
}
}  // namespace

int main() {
    probeFile("DOCKER_SOCK_VAR_RUN", "/var/run/docker.sock");
    probeFile("DOCKER_SOCK_RUN", "/run/docker.sock");
    probeFile("CANARY_FILE", LEETKOB_CANARY_PATH);

    FILE* mountinfo = std::fopen("/proc/self/mountinfo", "r");
    if (mountinfo == nullptr) {
        std::printf("MOUNTINFO=unreadable\n");
        return 0;
    }
    std::printf("MOUNTINFO_BEGIN\n");
    char buffer[65536];
    size_t total = 0;
    size_t read = 0;
    int lines = 0;
    while ((read = std::fread(buffer, 1, sizeof(buffer), mountinfo)) > 0) {
        std::fwrite(buffer, 1, read, stdout);
        total += read;
        for (size_t i = 0; i < read; ++i) {
            if (buffer[i] == '\n') {
                ++lines;
            }
        }
    }
    std::fclose(mountinfo);
    std::printf("\nMOUNTINFO_END\n");
    std::printf("MOUNTINFO_BYTES=%zu MOUNTINFO_LINES=%d\n", total, lines);
    return 0;
}
