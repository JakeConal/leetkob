#include <fstream>
#include <iostream>
#include <string>

// Compile-time inclusion probe. The canary file is created only on the host
// by the test. __has_include is true only when that file is visible inside
// the compilation container (a host mount or a copied secret).
#if __has_include("/host-canary/canary.txt")
constexpr int includeHit = 1;
#else
constexpr int includeHit = 0;
#endif

extern char** environ;

int main() {
  std::cout << "include_hit=" << includeHit << "\n";
  bool leaked = false;
  for (char** env = environ; env != nullptr && *env != nullptr; env++) {
    const std::string entry(*env);
    if (entry.find("MODEL_API_KEY") != std::string::npos ||
        entry.find("OPENAI_API_KEY") != std::string::npos ||
        entry.find("ANTHROPIC_API_KEY") != std::string::npos ||
        entry.find("LEETKOB_SYNTHETIC_CANARY") != std::string::npos) {
      std::cout << "env " << entry << "\n";
      leaked = true;
    }
  }
  if (!leaked) {
    std::cout << "env_clean\n";
  }
  const char* paths[] = {
      "/host-canary/canary.txt",
      "/run/secrets/model_api_key",
      "/etc/leetkob/model_api_key",
      "/opt/leetkob/model_api_key",
      "/tmp/model_api_key",
  };
  bool opened = false;
  for (const char* path : paths) {
    std::ifstream in(path);
    if (!in) {
      continue;
    }
    std::string line;
    std::getline(in, line);
    std::cout << "file=" << line << "\n";
    opened = true;
  }
  if (!opened) {
    std::cout << "file_absent\n";
  }
  std::cout << "secret_probe_done\n" << std::flush;
  return 0;
}
