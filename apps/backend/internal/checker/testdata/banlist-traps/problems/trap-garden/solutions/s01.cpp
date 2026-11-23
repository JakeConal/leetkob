#include <algorithm>
#include <iostream>
#include <vector>

int main() {
  std::vector<int> values{3, 1, 2};
  std::sort(values.begin(), values.end());
  std::cout << values.front() << '\n';
  return 0;
}
