// Attack: fork and thread ceiling. Child processes and worker threads each
// hold a slot for two seconds, so the PID/thread cgroup limit is reached and
// creation fails with EAGAIN. Expected containment: at least one failure
// below the configured limit, and the run still terminates within the wall.
#include <pthread.h>
#include <unistd.h>

#include <cerrno>
#include <cstdio>
#include <cstring>

namespace {
void* idle(void*) {
    sleep(2);
    return nullptr;
}
}  // namespace

int main() {
    std::setvbuf(stdout, nullptr, _IONBF, 0);
    int threads = 0;
    for (int i = 0; i < 4096; ++i) {
        pthread_t thread{};
        int rc = pthread_create(&thread, nullptr, idle, nullptr);
        if (rc != 0) {
            std::printf("THREAD_FAILED_AT=%d ERR=%s\n", i, std::strerror(rc));
            break;
        }
        ++threads;
    }
    if (threads == 4096) {
        std::printf("THREAD_FAILED_AT=none\n");
    }

    int children = 0;
    for (int i = 0; i < 4096; ++i) {
        pid_t child = fork();
        if (child == 0) {
            sleep(2);
            _exit(0);
        }
        if (child < 0) {
            std::printf("FORK_FAILED_AT=%d ERR=%s\n", i, std::strerror(errno));
            break;
        }
        ++children;
    }
    if (children == 4096) {
        std::printf("FORK_FAILED_AT=none\n");
    }
    std::printf("THREADS=%d CHILDREN=%d\n", threads, children);
    return 0;
}
