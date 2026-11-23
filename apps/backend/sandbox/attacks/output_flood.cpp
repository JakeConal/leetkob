// Attack: flood stdout past the collection cap. Expected containment: the
// controller stores at most the cap, keeps draining so the submission is
// not deadlocked, and maps the overflow to RE.
#include <cstdio>
#include <cstring>

int main() {
    char line[1024];
    std::memset(line, 'A', sizeof(line));
    line[sizeof(line) - 1] = '\n';
    for (int i = 0; i < 16384; ++i) {
        if (std::fwrite(line, 1, sizeof(line), stdout) != sizeof(line)) {
            return 2;
        }
    }
    std::printf("FLOOD_DONE=1\n");
    return 0;
}
