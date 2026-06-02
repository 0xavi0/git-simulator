// Command gitsim runs the git-simulator as a standalone HTTP server.
// It serves git smart-HTTP (ls-remote + clone), vendor REST APIs, and a
// control plane for driving scenarios from Fleet's e2e tests or curl.
//
// Usage:
//
//	gitsim [--listen :8080] [--vendors github] [--git-base-url http://host:port] [--content-dir ./manifests]
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/gitsim"
	_ "github.com/rancher/gitsim/pkg/provider/github" // register GitHub provider
)

func main() {
	listen := flag.String("listen", ":8080", "TCP address to listen on")
	vendors := flag.String("vendors", "github", "comma-separated list of enabled vendors")
	gitBaseURL := flag.String("git-base-url", "",
		"base URL used to build clone/webhook URLs (default: http://localhost:PORT)")
	contentDir := flag.String("content-dir", "",
		"host directory to serve as repository content; default serves a built-in README.md stub")
	logLevel := flag.String("log-level", "info", "log level (info, debug)")
	flag.Parse()

	_ = *logLevel // placeholder; wire a real logger when needed

	vendorList := splitTrimmed(*vendors)

	var opts []gitsim.Option
	opts = append(opts, gitsim.WithProviders(vendorList...))
	if *contentDir != "" {
		opts = append(opts, gitsim.WithContent(core.NewLocalDirContent(*contentDir)))
	}
	sim := gitsim.New(opts...)

	baseURL := *gitBaseURL
	if baseURL == "" {
		_, port, _ := net.SplitHostPort(*listen)
		if port == "" {
			port = "8080"
		}
		baseURL = "http://localhost:" + port
	}
	sim.SetBaseURL(baseURL)

	fmt.Printf("gitsim  listen   : %s\n", *listen)
	fmt.Printf("        base URL : %s\n", baseURL)
	fmt.Printf("        vendors  : %s\n", strings.Join(vendorList, ", "))
	if *contentDir != "" {
		fmt.Printf("        content  : %s\n", *contentDir)
	} else {
		fmt.Println("        content  : built-in README.md stub (use --content-dir to override)")
	}
	fmt.Println("        routes:")
	fmt.Println("          GET  /healthz")
	fmt.Println("          POST /control/repos")
	fmt.Println("          POST /control/repos/{vendor}/{owner}/{repo}/commits")
	fmt.Println("          POST /control/repos/{vendor}/{owner}/{repo}/webhooks")
	fmt.Println("          GET  /{owner}/{repo}.git/info/refs  (ls-remote)")
	fmt.Println("          POST /{owner}/{repo}.git/git-upload-pack  (clone)")

	srv := &http.Server{
		Addr:    *listen,
		Handler: sim.Handler(),
	}
	log.Fatal(srv.ListenAndServe())
}

func splitTrimmed(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}
