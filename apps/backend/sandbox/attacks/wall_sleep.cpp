// Attack: wall-clock sleep. Expected containment: the host-enforced wall
// deadline kills the container (TLE); SLEPT is never printed.
#include <unistd.h>

#include <cstdio>

int main() {
    sleep(60);
    std::printf("SLEPT=done\n");
    return 0;
}
