#include <algorithm>
#include <vector>

int main() {
  std::vector<int> values{2, 1};
  std::sort(values.begin(), values.end());  // marker-direct
  return 0;
}
