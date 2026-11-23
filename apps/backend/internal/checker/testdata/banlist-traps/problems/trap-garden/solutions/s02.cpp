#include <string>

// std::sort is mentioned here but never called
int main() {
  std::string note = "std::sort and malloc are only text";
  return static_cast<int>(note.size());
}
