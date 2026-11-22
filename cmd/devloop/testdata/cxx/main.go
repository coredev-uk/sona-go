// Command cxx is TestTargetLinker's cgo program with a C++ file (answer.cc):
// go build links it with the C++ compiler.
package main

// int answer(void);
import "C"

func main() { _ = C.answer() }
