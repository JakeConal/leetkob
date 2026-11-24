#include <array>
#include <iostream>
#include <string>

// A local helper named sort must not be mistaken for the banned library call.
static std::string sort(const std::string& text) {
  std::array<int, 256> counts{};
  for (unsigned char c : text) counts[c]++;
  std::string ordered;
  for (int c = 0; c < 256; ++c) ordered.append(counts[c], static_cast<char>(c));
  return ordered;
}

int main() {
  std::string line;
  std::getline(std::cin, line);
  std::cout << sort(line) << '\n';
  return 0;
}
