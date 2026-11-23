// Attack: outbound network (DNS resolution and a direct TCP connect).
// Expected containment: both attempts fail inside `--network none`; the
// program itself exits 0 and prints the denial markers.
#include <arpa/inet.h>
#include <netdb.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>

#include <cerrno>
#include <cstdio>
#include <cstring>

int main() {
    addrinfo hints{};
    hints.ai_family = AF_INET;
    hints.ai_socktype = SOCK_STREAM;
    addrinfo* resolved = nullptr;
    int rc = getaddrinfo("example.com", "80", &hints, &resolved);
    std::printf("DNS=%s\n", rc == 0 ? "resolved" : "denied");
    if (resolved != nullptr) {
        freeaddrinfo(resolved);
    }

    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) {
        std::printf("SOCKET=denied ERR=%s\n", std::strerror(errno));
        return 0;
    }
    sockaddr_in target{};
    target.sin_family = AF_INET;
    target.sin_port = htons(53);
    inet_pton(AF_INET, "1.1.1.1", &target.sin_addr);
    rc = connect(fd, reinterpret_cast<sockaddr*>(&target), sizeof(target));
    std::printf("CONNECT=%s\n", rc == 0 ? "connected" : "denied");
    if (rc == 0) {
        const char probe[] = "leetkob-network-probe";
        (void)send(fd, probe, sizeof(probe) - 1, 0);
    }
    close(fd);
    return 0;
}
