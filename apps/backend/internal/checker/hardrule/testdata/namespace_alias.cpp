#include <algorithm>
#include <vector>

namespace library = std;

int main() {
  std::vector<int> values{2, 1};
  library::sort(values.begin(), values.end());  // marker-alias
  return 0;
}
