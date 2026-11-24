#include <cctype>
#include <cstdlib>
#include <cstring>
#include <iostream>

int main() {
  char line[256];
  std::cin.getline(line, sizeof line);
  char* copy = static_cast<char*>(malloc(std::strlen(line) + 1));
  std::strcpy(copy, line);
  for (char* p = copy; *p; ++p) *p = static_cast<char>(std::toupper(static_cast<unsigned char>(*p)));
  std::cout << copy << '\n';
  std::free(copy);
  return 0;
}
