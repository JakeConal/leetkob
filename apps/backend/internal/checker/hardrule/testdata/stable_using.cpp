#include <algorithm>
#include <vector>

using std::stable_sort;

int main() {
  std::vector<int> values{2, 1};
  stable_sort(values.begin(), values.end());  // marker-stable
  return 0;
}
