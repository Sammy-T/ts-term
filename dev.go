package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

func startDevServer(pkgManager string) {
	log.Println("Starting Vite dev server...")

	cwDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Current working directory: %v", err)
	}

	cmdDir := filepath.Join(cwDir, "web")

	cmdInst := exec.Command(pkgManager, "i")
	cmdInst.Dir = cmdDir
	cmdInst.Stdout = os.Stdout
	cmdInst.Stderr = os.Stderr

	// Run install and await
	if err = cmdInst.Run(); err != nil {
		if pkgManager != "npm" {
			log.Printf("%v install failed: %v.\nFalling back to npm...", pkgManager, err)

			startDevServer("npm")
			return
		}

		log.Fatalf("%v install: %v", pkgManager, err)
	}

	cmd := exec.Command(pkgManager, "run", "dev")
	cmd.Dir = cmdDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run dev without awaiting
	if err = cmd.Start(); err != nil {
		log.Fatalf("Vite dev server: %v", err)
	}

	log.Printf("Vite dev server running as pid %v", cmd.Process.Pid)
}

func createDevHandler() http.Handler {
	errHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadGateway)

		w.Write([]byte("Error: " + err.Error()))
	}

	devUrl, err := url.Parse("http://localhost:5173")
	if err != nil {
		log.Fatalf("Dev url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(devUrl)
	proxy.ErrorHandler = errHandler

	return proxy
}
