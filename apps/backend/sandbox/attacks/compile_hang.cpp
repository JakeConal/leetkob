// Fixture: make the compiler preprocessor open and read an endless pty
// master, which blocks without allocating memory. The container has a
// host-enforced compile wall, so this must end as a CE with
// TimedOut = true, never as an infinite worker stall.
#include "/dev/ptmx"

int main() {
    return 0;
}
