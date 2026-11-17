// Command module is the WebAssembly add-on Sona's tests run. It is built for
// GOOS=wasip1 by package wasmtest, which is the only thing that builds it:
// it sits in testdata so no other build sees it. It speaks protocol v2 over
// stdin and stdout, as docs/addons.md says, and logs to stderr.
//
// It uses nothing but os and strings, by hand: a module without fmt and
// encoding/json is half the size, and a test compiles it under the race
// detector, where the size is what the time goes to.
//
// LC_WASMTEST_MODE picks what it does after start:
//
//	(none)  a well-behaved add-on: hello, then answers invokes and logs events
//	files   probes the files it can reach and logs each result
//	env     logs its environment
//	loop    spins forever
//	alloc   allocates until it can't
//	deaf    ignores its input and never leaves
//	exit    exits with status 3
package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func logf(s string) { _, _ = os.Stderr.WriteString(s + "\n") }

func main() {
	switch os.Getenv("LC_WASMTEST_MODE") {
	case "files":
		probeFiles()
	case "env":
		for _, kv := range os.Environ() {
			logf("env " + kv)
		}
		waitForEnd()
	case "loop":
		logf("looping")
		for {
		}
	case "alloc":
		alloc()
	case "deaf":
		logf("deaf")
		for {
			time.Sleep(5 * time.Millisecond)
		}
	case "exit":
		logf("exiting")
		os.Exit(3)
	default:
		addon()
	}
}

func waitForEnd() {
	buf := make([]byte, 4096)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			return
		}
	}
}

// readLine reads one line from stdin, without its newline; ok is false at
// the end of the input.
func readLine(pending *[]byte) (line string, ok bool) {
	buf := make([]byte, 4096)
	for {
		if i := strings.IndexByte(string(*pending), '\n'); i >= 0 {
			line = string((*pending)[:i])
			*pending = (*pending)[i+1:]
			return line, true
		}
		n, err := os.Stdin.Read(buf)
		*pending = append(*pending, buf[:n]...)
		if err != nil {
			if len(*pending) > 0 {
				line, *pending = string(*pending), nil
				return line, true
			}
			return "", false
		}
	}
}

func send(s string) {
	if _, err := os.Stdout.WriteString(s + "\n"); err != nil {
		logf("write: " + err.Error())
		os.Exit(1)
	}
}

// strField is the string after `"key":"` up to the next quote.
func strField(line, key string) string {
	_, rest, ok := strings.Cut(line, `"`+key+`":"`)
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, `"`)
	return v
}

// intField is the number after `"key":`, -1 when there is none.
func intField(line, key string) int {
	_, rest, ok := strings.Cut(line, `"`+key+`":`)
	if !ok {
		return -1
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	v, err := strconv.Atoi(rest[:n])
	if err != nil {
		return -1
	}
	return v
}

func quoteList(csv string) string {
	if csv == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(csv, ",", `","`) + `"`
}

func addon() {
	id := os.Getenv("SONA_ADDON_ID")
	send(`{"id":1,"method":"hello","params":{"id":"` + id + `","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":[` +
		quoteList(os.Getenv("LC_WASMTEST_PERMS")) + `]}}`)
	next := 1
	var pending []byte
	for {
		line, ok := readLine(&pending)
		if !ok {
			logf("input ended")
			return
		}
		switch event := strField(line, "event"); {
		case event == "shutdown":
			logf("shutdown")
			return
		case event != "":
			logf("event " + event)
		case strings.Contains(line, `"method":"invoke"`):
			action := strField(line, "action")
			logf("invoke " + action)
			send(`{"id":` + strconv.Itoa(intField(line, "id")) + `,"result":{"toast":"wasm says ` + action + `","url":"https://example.com/wasm"}}`)
		case strings.Contains(line, `"error":{"code":"`):
			logf("error " + strField(line, "code"))
		case intField(line, "id") == 1:
			logf("hello ok")
			if t := os.Getenv("LC_WASMTEST_TOPICS"); t != "" {
				next++
				send(`{"id":` + strconv.Itoa(next) + `,"method":"subscribe","params":{"topics":[` + quoteList(t) + `]}}`)
			}
		default:
			logf("answer " + strconv.Itoa(intField(line, "id")))
		}
	}
}

func probe(name string, err error) {
	if err != nil {
		logf("probe " + name + ": denied")
		return
	}
	logf("probe " + name + ": ok")
}

func probeFiles() {
	secret := os.Getenv("LC_WASMTEST_SECRET")
	_, err := os.ReadFile("/etc/passwd")
	probe("etc-passwd", err)
	_, err = os.ReadFile("/data/../../../../etc/passwd")
	probe("dotdot", err)
	_, err = os.ReadFile(secret)
	probe("secret-by-host-path", err)
	probe("data-write", os.WriteFile("/data/hello.txt", []byte("hi"), 0o600))
	b, err := os.ReadFile("/data/hello.txt")
	probe("data-read", err)
	if err == nil && string(b) != "hi" {
		logf("data-read gave " + string(b))
	}
	probe("data-mkdir", os.Mkdir("/data/sub", 0o700))
	probe("data-nested", os.WriteFile("/data/sub/x", []byte("x"), 0o600))
	probe("symlink-out", os.Symlink(secret, "/data/new-link"))
	probe("symlink-in", os.Symlink("hello.txt", "/data/inner-link"))
	_, err = os.ReadFile("/data/planted-link")
	probe("planted-link", err)
	_, err = os.ReadFile("/data/planted-dir/secret.txt")
	probe("planted-dir", err)
	waitForEnd()
}

func alloc() {
	var keep [][]byte
	mib := 0
	for {
		chunk := make([]byte, 1<<20)
		for i := range chunk {
			chunk[i] = 1
		}
		keep = append(keep, chunk)
		mib++
		if mib%4 == 0 {
			logf("alloc " + strconv.Itoa(mib) + " MiB")
		}
	}
}
