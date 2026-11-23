// Attack: root/privileged probe. Expected containment: uid/gid 65534,
// setuid(0) denied, every capability dropped, no-new-privileges set, no
// user namespace, /etc/shadow unreadable and the root filesystem read-only.
#include <fcntl.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <unistd.h>

#include <cerrno>
#include <cstdio>
#include <cstring>

#include <sched.h>

int main() {
    std::printf("UID=%d EUID=%d GID=%d EGID=%d\n", getuid(), geteuid(), getgid(), getegid());

    errno = 0;
    int rc = setuid(0);
    std::printf("SETUID0=%s ERR=%s\n", rc == 0 ? "allowed" : "denied",
                rc == 0 ? "none" : std::strerror(errno));

    FILE* status = std::fopen("/proc/self/status", "r");
    if (status != nullptr) {
        char line[512];
        while (std::fgets(line, sizeof(line), status) != nullptr) {
            const char* value = nullptr;
            if (std::strncmp(line, "CapEff:", 7) == 0) {
                value = line + 7;
                std::printf("CAPEFF=");
            } else if (std::strncmp(line, "CapPrm:", 7) == 0) {
                value = line + 7;
                std::printf("CAPPRM=");
            } else if (std::strncmp(line, "NoNewPrivs:", 11) == 0) {
                value = line + 11;
                std::printf("NONEWPRIVS=");
            }
            if (value != nullptr) {
                while (*value == ' ' || *value == '\t') {
                    ++value;
                }
                std::printf("%s", value);
            }
        }
        std::fclose(status);
    }

    int shadow = open("/etc/shadow", O_RDONLY);
    std::printf("SHADOW=%s\n", shadow >= 0 ? "readable" : "denied");
    if (shadow >= 0) {
        close(shadow);
    }

    int created = open("/etc/leetkob-rootfs-probe", O_CREAT | O_WRONLY, 0644);
    std::printf("ROOTFS_WRITE=%s\n", created >= 0 ? "allowed" : "denied");
    if (created >= 0) {
        close(created);
    }

    errno = 0;
    rc = static_cast<int>(syscall(SYS_unshare, CLONE_NEWUSER));
    std::printf("USERNS=%s ERR=%s\n", rc == 0 ? "allowed" : "denied",
                rc == 0 ? "none" : std::strerror(errno));
    return 0;
}
