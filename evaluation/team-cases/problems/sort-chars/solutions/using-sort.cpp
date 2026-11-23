#include <algorithm>
#include <iostream>
#include <string>

int main() {
  std::string line;
  std::getline(std::cin, line);
  using std::sort;
  sort(line.begin(), line.end());
  std::cout << line << '\n';
  return 0;
}
