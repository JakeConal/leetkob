#include <cstdlib>
#include <iostream>
#include <map>
#include <set>

int main() {
  std::set<int> seen;
  int value = 0;
  while (std::cin >> value) seen.insert(value);
  std::cout << seen.size() << '\n';
  std::exit(0);
}
