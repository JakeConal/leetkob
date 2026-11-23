#include <vector>

namespace student {
template <class It>
void sort(It first, It last) {
  if (first == last) {
    return;
  }
  for (It i = first + 1; i != last; ++i) {
    auto key = *i;
    It j = i;
    while (j != first && *(j - 1) > key) {
      *j = *(j - 1);
      --j;
    }
    *j = key;
  }
}
}  // namespace student

int main() {
  std::vector<int> values{3, 1, 2};
  student::sort(values.begin(), values.end());  // marker-own-sort
  return static_cast<int>(values.size());
}
