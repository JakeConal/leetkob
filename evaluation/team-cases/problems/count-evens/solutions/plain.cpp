#include <iostream>

int main() {
  int n = 0;
  if (!(std::cin >> n)) return 0;
  int evens = 0;
  for (int i = 0; i < n; ++i) {
    long long value = 0;
    std::cin >> value;
    if (value % 2 == 0) ++evens;
  }
  std::cout << evens << '\n';
  return 0;
}
