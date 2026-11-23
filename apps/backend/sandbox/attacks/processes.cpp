#include <cstdio>
#include <pthread.h>
#include <unistd.h>

static void* worker(void*) {
  pause();
  return nullptr;
}

// Ask for more threads than the ceiling of 16, which includes the main
// thread. Small stacks keep the probe from turning into a memory attack.
int main() {
  pthread_attr_t attr;
  pthread_attr_init(&attr);
  pthread_attr_setstacksize(&attr, 64 * 1024);
  int created = 0;
  for (int i = 0; i < 64; i++) {
    pthread_t thread;
    if (pthread_create(&thread, &attr, worker, nullptr) != 0) {
      break;
    }
    created++;
  }
  pthread_attr_destroy(&attr);
  std::printf("created=%d\n", created);
  std::fflush(stdout);
  return 0;
}
