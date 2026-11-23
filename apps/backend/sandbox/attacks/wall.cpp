#include <chrono>
#include <cstdlib>
#include <iostream>
#include <thread>

// Block without burning CPU. The host deadline must remove the container
// before WALL_FINISHED. The argument is the sleep in seconds.
int main(int argc, char** argv) {
  int seconds = 30;
  if (argc > 1) {
    seconds = std::atoi(argv[1]);
  }
  if (seconds < 0) {
    seconds = 0;
  }
  std::this_thread::sleep_for(std::chrono::seconds(seconds));
  std::cout << "WALL_FINISHED\n" << std::flush;
  return 0;
}
