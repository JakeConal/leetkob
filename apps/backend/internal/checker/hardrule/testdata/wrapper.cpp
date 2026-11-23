#include <algorithm>
#include <vector>

template <class It>
void order(It first, It last) {
  std::sort(first, last);  // marker-wrapper
}

int main() {
  std::vector<int> values{2, 1};
  order(values.begin(), values.end());  // marker-wrapper-call
  return 0;
}
