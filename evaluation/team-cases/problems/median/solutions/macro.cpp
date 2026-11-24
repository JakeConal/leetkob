#include <algorithm>
#include <iostream>
#include <vector>

#define ORDER std::sort

int main() {
  int n = 0;
  if (!(std::cin >> n)) return 0;
  std::vector<long long> values(n);
  for (auto& value : values) std::cin >> value;
  ORDER(values.begin(), values.end());
  std::cout << values[n / 2] << '\n';
  return 0;
}
