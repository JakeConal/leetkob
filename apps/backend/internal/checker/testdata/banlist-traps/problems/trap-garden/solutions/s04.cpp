#include <algorithm>
#include <vector>

using std::sort;

int main() {
  std::vector<int> values{2, 1};
  sort(values.begin(), values.end());
  return values.front();
}
