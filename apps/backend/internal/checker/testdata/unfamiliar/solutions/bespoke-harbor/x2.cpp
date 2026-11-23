#include <iostream>

int main() {
  int left = 0;
  int right = 0;
  std::cin >> left >> right;
  int result = right;
  if (left > right) goto choose_left;
  goto done;
choose_left:
  result = left;
done:
  std::cout << result << '\n';
  return 0;
}
