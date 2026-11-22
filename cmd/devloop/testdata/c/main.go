// Command c is TestTargetLinker's cgo program with no C++: go build links
// it with the C compiler.
package main

// static int answer(void) { return 42; }
import "C"

func main() { _ = C.answer() }
