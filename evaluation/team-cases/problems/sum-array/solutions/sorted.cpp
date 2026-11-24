#include <algorithm>
#include <iostream>
#include <vector>

int main() {
  int n = 0;
  if (!(std::cin >> n)) return 0;
  std::vector<long long> values(n);
  for (auto& value : values) std::cin >> value;
  std::sort(values.begin(), values.end());
  long long sum = 0;
  for (long long value : values) sum += value;
  std::cout << sum << '\n';
  return 0;
}
