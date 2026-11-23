#include <fstream>
#include <iostream>
#include <string>
#include <unistd.h>

// Host-only canary and the Docker socket. Neither path exists in the image.
// A bind mount of a host canary directory makes file= succeed; that is the
// negative control. The socket and docker client must stay absent.
int main() {
  const int socketOpen = access("/var/run/docker.sock", F_OK) == 0 ? 1 : 0;
  const int dockerCli = access("/usr/bin/docker", X_OK) == 0 ? 1 : 0;
  std::cout << "socket_open=" << socketOpen << "\n";
  std::cout << "docker_cli=" << dockerCli << "\n";
  std::ifstream in("/host-canary/canary.txt");
  if (in) {
    std::string line;
    std::getline(in, line);
    std::cout << "file=" << line << "\n";
  } else {
    std::cout << "file_absent\n";
  }
  std::cout << "mount_probe_done\n" << std::flush;
  return 0;
}
