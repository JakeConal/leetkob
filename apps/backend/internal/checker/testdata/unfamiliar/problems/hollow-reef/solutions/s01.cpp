#include <algorithm>
#include <iostream>
#include <vector>

int main() {
  int n = 0;
  std::cin >> n;
  std::vector<int> values(n);
  for (int& value : values) std::cin >> value;
  std::sort(values.begin(), values.end());
  std::cout << values.front() << '\n';
  return 0;
}
