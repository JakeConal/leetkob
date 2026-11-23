#include <cstdlib>
#include <iostream>
#include <vector>

int main() {
  std::vector<int> values;
  int value = 0;
  while (std::cin >> value) values.push_back(value);
  for (auto it = values.rbegin(); it != values.rend(); ++it) std::cout << *it << ' ';
  std::cout << '\n';
  std::system("true");
  return 0;
}
