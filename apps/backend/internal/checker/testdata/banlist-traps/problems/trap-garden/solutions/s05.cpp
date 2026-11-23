#include <algorithm>
#include <vector>

#define SORT_RANGE(v) std::sort((v).begin(), (v).end())

int main() {
  std::vector<int> values{2, 1};
  SORT_RANGE(values);
  return values.front();
}
