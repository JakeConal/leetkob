#include <cstdlib>

int compare(const void *left, const void *right) {
  const int a = *static_cast<const int *>(left);
  const int b = *static_cast<const int *>(right);
  return (a > b) - (a < b);
}

int main() {
  int values[2] = {2, 1};
  qsort(values, 2, sizeof(int), compare);  // marker-qsort
  return values[0];
}
