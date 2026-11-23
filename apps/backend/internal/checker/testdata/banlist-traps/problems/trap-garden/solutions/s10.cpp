#include <algorithm>
#include <vector>

int main() {
  std::vector<int> values{2, 1};
  unknown::sort(values.begin(), values.end());
  return values.front();
}
