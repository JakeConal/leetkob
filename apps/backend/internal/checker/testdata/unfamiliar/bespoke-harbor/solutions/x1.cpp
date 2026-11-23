#include <iostream>

int main() {
  int left = 0;
  int right = 0;
  std::cin >> left >> right;
  std::cout << (left > right ? left : right) << '\n';
  return 0;
}
