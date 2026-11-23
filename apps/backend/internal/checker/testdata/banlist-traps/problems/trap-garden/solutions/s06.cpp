#include <cstdlib>
#include <iostream>

int main() {
  int* values = static_cast<int*>(malloc(sizeof(int) * 4));
  if (values == nullptr) return 1;
  values[0] = 7;
  std::cout << values[0] << '\n';
  free(values);
  return 0;
}
