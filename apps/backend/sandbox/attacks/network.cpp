#include <arpa/inet.h>
#include <chrono>
#include <cstdio>
#include <cstring>
#include <iostream>
#include <netdb.h>
#include <string>
#include <sys/socket.h>
#include <thread>
#include <unistd.h>

// Probe a controlled endpoint. Numeric addresses avoid a DNS hang when the
// network namespace has no resolver. Retries give a just-started fixture a
// moment to accept; an isolated namespace fails each attempt immediately.
int main(int argc, char** argv) {
  const char* host = argc > 1 ? argv[1] : "203.0.113.5";
  const char* port = argc > 2 ? argv[2] : "9";
  for (int attempt = 0; attempt < 10; attempt++) {
    addrinfo hints{};
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_flags = AI_NUMERICHOST | AI_NUMERICSERV;
    addrinfo* res = nullptr;
    if (getaddrinfo(host, port, &hints, &res) != 0) {
      std::cout << "network_denied dns\n" << std::flush;
      return 0;
    }
    const int fd = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
    bool open = false;
    if (fd >= 0) {
      timeval tv{};
      tv.tv_sec = 1;
      setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof tv);
      open = connect(fd, res->ai_addr, res->ai_addrlen) == 0;
      close(fd);
    }
    freeaddrinfo(res);
    if (open) {
      std::cout << "network_open\n" << std::flush;
      return 0;
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  std::cout << "network_denied\n" << std::flush;
  return 0;
}
