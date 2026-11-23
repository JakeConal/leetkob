#include <algorithm>
#include <vector>

namespace alg = std;

int main() {
  std::vector<int> values{2, 1};
  alg::sort(values.begin(), values.end());
  return values.front();
}
