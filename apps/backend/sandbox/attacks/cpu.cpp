#include <chrono>
#include <cstdint>
#include <iostream>

// Busy loop. A working CPU-time limit kills the process before the marker.
// If the limit is omitted, the marker is printed and the process exits.
int main() {
  const auto start = std::chrono::steady_clock::now();
  volatile std::uint64_t x = 0;
  for (;;) {
    x++;
    const auto elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::steady_clock::now() - start);
    if (elapsed.count() >= 4000) {
      std::cout << "CPU_LIMIT_DISABLED\n" << std::flush;
      return 0;
    }
  }
}
