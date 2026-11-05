//go:build ignore

// gosources writes go-sources.json, the flatpak-builder source list that makes
// Sona's Go modules available to an offline build. Flatpak builds have no
// network, so each module's .mod and .zip is a checksummed "file"
// source laid out as a GOPROXY=file:// tree (go-proxy/<module>/@v/<version>.*),
// which the manifest's build commands point GOPROXY at.
//
// Regenerate after any go.mod or go.sum change (from the repository root,
// inside devenv so the module cache is warm and go matches go.mod):
//
//	go run build/linux/flatpak/gosources.go > build/linux/flatpak/go-sources.json
//
// The sha256 of each file is taken from the local module cache, which holds the
// bytes proxy.golang.org served; go.sum still verifies the module contents at
// build time. The .info files are left out: the proxy does not serve them
// byte for byte, and go build does not read them.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type module struct {
	Path, Version, GoMod, Zip string
}

type source struct {
	Type         string `json:"type"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Dest         string `json:"dest"`
	DestFilename string `json:"dest-filename"`
}

func main() {
	log.SetFlags(0)
	// GOFLAGS is cleared so the build tags devenv sets do not reach go mod.
	cmd := exec.Command("go", "mod", "download", "-json")
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	var sources []source
	dec := json.NewDecoder(out)
	for {
		var m module
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		for _, file := range []string{m.GoMod, m.Zip} {
			if file == "" {
				log.Fatalf("%s@%s: module is not in the module cache", m.Path, m.Version)
			}
			// .../cache/download/<escaped module>/@v/<version>.<ext>
			rel := file[strings.Index(file, "/cache/download/")+len("/cache/download/"):]
			dir, name := filepath.Split(rel)
			sum, err := hashFile(file)
			if err != nil {
				log.Fatal(err)
			}
			sources = append(sources, source{
				Type:         "file",
				URL:          "https://proxy.golang.org/" + rel,
				SHA256:       sum,
				Dest:         "go-proxy/" + strings.TrimSuffix(dir, "/"),
				DestFilename: name,
			})
		}
	}
	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sources); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "gosources: %d files\n", len(sources))
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
