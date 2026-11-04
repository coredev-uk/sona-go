//go:build probe

package main

// A probe build links MyGo's automation plugin: with MYGO_AUTOMATION set to
// a socket path, another process (cmd/uiprobe, the e2e tests) drives the app
// through MyGo's accessibility tree, input and capture. It also keeps
// MyGo's inspector, which automation needs: never combine `probe` with
// `mygo_noinspector`, and never ship a probe build.
import _ "github.com/egoist/mygo/plugins/automation"
