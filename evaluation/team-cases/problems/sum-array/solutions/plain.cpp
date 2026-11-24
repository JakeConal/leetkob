#include <iostream>

int main() {
  int n = 0;
  if (!(std::cin >> n)) return 0;
  long long sum = 0;
  for (int i = 0; i < n; ++i) {
    long long value = 0;
    std::cin >> value;
    sum += value;
  }
  std::cout << sum << '\n';
  return 0;
}
