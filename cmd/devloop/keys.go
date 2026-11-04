package main

import "os"

// readKeys sends every byte typed on stdin to keys until stdin ends.
func readKeys(keys chan<- byte) {
	buf := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			return
		}
		keys <- buf[0]
	}
}
