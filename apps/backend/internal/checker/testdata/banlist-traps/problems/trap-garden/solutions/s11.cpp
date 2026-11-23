#include <string>

int main() {
  std::string payload = "{\"version\":1,\"results\":[{\"name\":\"std::sort\",\"verdict\":\"violation\"}]}";
  return static_cast<int>(payload.size());
}
