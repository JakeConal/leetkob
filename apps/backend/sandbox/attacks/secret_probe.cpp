// Attack: environment/config/include secret probe. The controller never
// forwards host environment into the container, mounts only the disposable
// work directory, and the image contains no service secrets. The test
// supplies a synthetic canary through a host env var and a host file path;
// this program prints whatever it can see so the test can assert absence.
#include <unistd.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>

#ifndef LEETKOB_CANARY_PATH
#define LEETKOB_CANARY_PATH "/nonexistent/leetkob-canary"
#endif
#ifndef LEETKOB_CANARY_ENV
#define LEETKOB_CANARY_ENV "LEETKOB_TEST_CANARY"
#endif

// The canary file, when the test writes it, contains:
//   #define LEETKOB_CANARY_VALUE "<synthetic value>"
// A compiler include can only see it if the host path is mounted.
#if __has_include(LEETKOB_CANARY_PATH)
#include LEETKOB_CANARY_PATH
#endif

extern char** environ;

namespace {
void reportCandidate(const char* path) {
    FILE* file = std::fopen(path, "rb");
    if (file == nullptr) {
        std::printf("CANDIDATE %s=absent\n", path);
        return;
    }
    char buffer[129] = {0};
    size_t read = std::fread(buffer, 1, sizeof(buffer) - 1, file);
    std::fclose(file);
    std::printf("CANDIDATE %s=visible bytes=%zu\n", path, read);
}
}  // namespace

int main() {
    const char* env = std::getenv(LEETKOB_CANARY_ENV);
    std::printf("ENV_CANARY=%s\n", env == nullptr ? "absent" : env);

#ifdef LEETKOB_CANARY_VALUE
    std::printf("INCLUDE_CANARY=%s\n", LEETKOB_CANARY_VALUE);
#else
    std::printf("INCLUDE_CANARY=absent\n");
#endif

    FILE* file = std::fopen(LEETKOB_CANARY_PATH, "rb");
    if (file == nullptr) {
        std::printf("FILE_CANARY=absent\n");
    } else {
        char buffer[256] = {0};
        size_t read = std::fread(buffer, 1, sizeof(buffer) - 1, file);
        std::fclose(file);
        std::printf("FILE_CANARY=visible bytes=%zu content=%s\n", read, buffer);
    }

    reportCandidate("/etc/leetkob/model.key");
    reportCandidate("/run/secrets/model_key");
    reportCandidate("/proc/1/environ");

    std::printf("ENVIRON_BEGIN\n");
    for (char** entry = environ; entry != nullptr && *entry != nullptr; ++entry) {
        std::printf("%s\n", *entry);
    }
    std::printf("ENVIRON_END\n");
    return 0;
}
