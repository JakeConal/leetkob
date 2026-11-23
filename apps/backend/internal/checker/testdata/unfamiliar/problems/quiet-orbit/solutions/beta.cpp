#include <iostream>
#include <unordered_set>

int main() {
  std::unordered_set<int> seen;
  int value = 0;
  while (std::cin >> value) seen.insert(value);
  std::cout << seen.size() << '\n';
  return 0;
}
