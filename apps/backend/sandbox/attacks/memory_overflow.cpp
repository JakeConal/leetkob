// Attack: allocate well past the memory/swap ceiling, write every page, and
// read the written values back so the compiler cannot optimise the
// allocation away.
// Expected containment: the cgroup OOM killer ends the container. Docker
// reports OOMKilled, so the controller maps this to MLE instead of RE.
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

int main() {
    std::setvbuf(stdout, nullptr, _IONBF, 0);
    const size_t chunk = 8u * 1024u * 1024u;
    std::vector<char*> blocks;

    for (int i = 0; i < 256; ++i) {
        char* block = static_cast<char*>(std::malloc(chunk));
        if (block == nullptr) {
            std::printf("MALLOC_FAILED_AT=%d\n", i);
            return 0;
        }
        std::memset(block, 0xA5, chunk);
        blocks.push_back(block);

        volatile unsigned long long sum = 0;
        for (size_t offset = 0; offset < chunk; offset += 4096) {
            sum += static_cast<unsigned char>(block[offset]);
        }
        std::printf("ALLOC=%d READBACK=%llu\n", i, sum);
    }
    std::printf("ALLOCATION_COMPLETED=unexpected\n");
    return 1;
}
