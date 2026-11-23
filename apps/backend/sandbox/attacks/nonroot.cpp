#include <cstdio>
#include <unistd.h>

int main() {
  const unsigned long euid = static_cast<unsigned long>(geteuid());
  std::printf("euid=%lu\n", euid);
  if (seteuid(0) == 0) {
    std::printf("priv_esc=1\n");
  } else {
    std::printf("priv_esc=0\n");
  }
  std::fflush(stdout);
  return 0;
}
