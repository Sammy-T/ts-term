package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	var dev bool
	var handler http.Handler

	flag.BoolVar(&dev, "dev", false, "development mode")

	flag.Parse()

	if dev {
		startDevServer()
		handler = createDevHandler()
	} else {
		handler = http.FileServer(http.Dir("web/dist"))
	}

	log.Println("Starting Go server...")

	http.Handle("/", handler)

	log.Fatal(http.ListenAndServe(":3000", nil))
}

func startDevServer() {
	log.Println("Starting Vite dev server...")

	cwDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Current working directory: %v", err)
	}

	cmd := exec.Command("pnpm", "run", "dev")
	cmd.Dir = filepath.Join(cwDir, "web")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		log.Fatalf("Vite dev server: %v", err)
	}

	log.Printf("Vite dev server running as pid %v\n", cmd.Process.Pid)
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
