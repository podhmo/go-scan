package main

import oddpkg "github.com/podhmo/go-scan/minigo2/testdata/oddname"

// Use references the package by its declared name (oddpkg), not the
// import path's last element (oddname).
func Use() int { return oddpkg.Magic() } // 7

func main() {}
