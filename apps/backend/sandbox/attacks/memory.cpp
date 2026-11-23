#include <cstddef>
#include <cstdint>
#include <fstream>
#include <iostream>
#include <new>
#include <string>
#include <vector>

// Allocate past the smallest legal memory ceiling (64 MiB) and read every
// page back so the compiler cannot delete the store. 96 MiB still fits in
// the 512 MiB ceiling, which is what makes a disabled limit observable.
int main() {
  for (const char* path : {"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.swap.max"}) {
    std::ifstream in(path);
    std::string value;
    if (!std::getline(in, value)) {
      value = "unreadable";
    }
    std::cout << path << "=" << value << "\n";
  }
  std::cout << std::flush;
  constexpr std::size_t bytes = 96u << 20;
  try {
    std::vector<unsigned char> buf(bytes);
    volatile unsigned char sink = 0;
    std::uint64_t sum = 0;
    for (std::size_t i = 0; i < bytes; i += 4096) {
      buf[i] = static_cast<unsigned char>(i & 0xff);
      sink = buf[i];
      sum += sink;
    }
    std::uint64_t back = 0;
    for (std::size_t i = 0; i < bytes; i += 4096) {
      sink = buf[i];
      back += sink;
    }
    if (back != sum) {
      return 3;
    }
    std::cout << "readback_ok " << back << "\n" << std::flush;
  } catch (const std::bad_alloc&) {
    std::cout << "alloc_failed\n" << std::flush;
    return 1;
  }
  return 0;
}
