#include <iostream>
#include <string>

// Write well past a few-kilobyte collection cap, then a marker. Bounded
// collection keeps the marker out of the captured output and ends the run.
// A cap above 32 KiB lets the marker through, so a disabled cap is visible.
int main() {
  const std::string block(1024, 'B');
  for (int i = 0; i < 32; i++) {
    std::cout << block;
  }
  std::cout << "OUTPUT_DONE\n" << std::flush;
  return 0;
}
