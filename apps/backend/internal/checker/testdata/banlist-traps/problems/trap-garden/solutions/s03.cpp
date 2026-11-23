void sort(int* begin, int* end) {
  for (int* i = begin; i != end; ++i)
    for (int* j = i + 1; j != end; ++j)
      if (*j < *i) {
        int tmp = *i;
        *i = *j;
        *j = tmp;
      }
}

int main() {
  int values[2] = {2, 1};
  sort(values, values + 2);
  return values[0];
}
