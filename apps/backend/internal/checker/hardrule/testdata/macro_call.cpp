#include <algorithm>
#include <vector>

#define ORDER std::sort

int main() {
  std::vector<int> values{2, 1};
  ORDER(values.begin(), values.end());  // marker-macro
  return 0;
}
