#include <cstdlib>
#include <iostream>

int main() {
  int* values = static_cast<int*>(std::malloc(4 * sizeof(int)));
  values[0] = 0;
  std::cout << values[0] << '\n';
  std::free(values);
  return 0;
}
