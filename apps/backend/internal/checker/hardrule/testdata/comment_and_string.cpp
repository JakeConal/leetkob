#include <vector>

// The token std::sort is only a comment, not a call.
int main() {
  const char *note = "std::sort";  // marker-string
  std::vector<int> values{1};
  (void)note;
  return static_cast<int>(values.size());
}
