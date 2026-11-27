#include <iostream>
#include <vector>

void merge_sort(std::vector<int>& a, int lo, int hi) {
    if (hi - lo < 2) return;
    int mid = (lo + hi) / 2;
    merge_sort(a, lo, mid);
    merge_sort(a, mid, hi);
    std::vector<int> merged;
    int i = lo, j = mid;
    while (i < mid || j < hi) {
        if (j == hi || (i < mid && a[i] <= a[j])) merged.push_back(a[i++]);
        else merged.push_back(a[j++]);
    }
    for (int k = lo; k < hi; ++k) a[k] = merged[k - lo];
}

int main() {
    int n;
    if (!(std::cin >> n)) return 1;
    std::vector<int> a(n);
    for (int& x : a) std::cin >> x;
    merge_sort(a, 0, n);
    for (int x : a) std::cout << x << ' ';
    std::cout << '\n';
}
