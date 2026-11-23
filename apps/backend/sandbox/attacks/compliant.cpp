// Compliant C++17 control program: reads two integers and prints their sum.
// Used by the positive test to prove the fixed image compiles and runs a
// normal submission and returns token data for later comparison.
#include <iostream>

int main() {
    long long a = 0;
    long long b = 0;
    if (!(std::cin >> a >> b)) {
        return 1;
    }
    std::cout << (a + b) << '\n';
    return 0;
}
