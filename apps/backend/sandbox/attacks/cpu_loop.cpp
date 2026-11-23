// Attack: unbounded CPU loop. Expected containment: the CPU quota bounds
// host consumption and the host wall deadline kills the container (TLE).
int main() {
    volatile unsigned long long counter = 0;
    for (;;) {
        counter = counter + 1;
    }
}
