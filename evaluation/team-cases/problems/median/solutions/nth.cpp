#include <algorithm>
#include <iostream>
#include <vector>

int main() {
  int n = 0;
  if (!(std::cin >> n)) return 0;
  std::vector<long long> values(n);
  for (auto& value : values) std::cin >> value;
  std::nth_element(values.begin(), values.begin() + n / 2, values.end());
  std::cout << values[n / 2] << '\n';
  return 0;
}
