#include <iostream>
#include <utility>

int main() {
  int left = 0;
  int right = 0;
  std::cin >> left >> right;
  if (left > right) std::swap(left, right);
  std::cout << left << ' ' << right << '\n';
  return 0;
}
