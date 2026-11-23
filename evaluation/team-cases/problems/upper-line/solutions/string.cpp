#include <cctype>
#include <iostream>
#include <string>

int main() {
  std::string line;
  std::getline(std::cin, line);
  for (char& c : line) c = static_cast<char>(std::toupper(static_cast<unsigned char>(c)));
  std::cout << line << '\n';
  return 0;
}
