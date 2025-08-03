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

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const bufferSize int = 1024

var upgrader = websocket.Upgrader{
	ReadBufferSize:  bufferSize,
	WriteBufferSize: bufferSize,
}

func main() {
	var dev bool

	flag.BoolVar(&dev, "dev", false, "development mode")

	flag.Parse()

	handler := getHandler(dev)

	log.Println("Starting Go server...")

	http.Handle("/", handler)
	http.HandleFunc("/term", wsHandler)

	log.Fatal(http.ListenAndServe(":3000", nil))
}

func getHandler(dev bool) http.Handler {
	if dev {
		startDevServer()
		return createDevHandler()
	}

	return http.FileServer(http.Dir("web/dist"))
}

func startDevServer() {
	log.Println("Starting Vite dev server...")

	cwDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Current working directory: %v", err)
	}

	cmdDir := filepath.Join(cwDir, "web")

	cmdInst := exec.Command("npm", "i")
	cmdInst.Dir = cmdDir
	cmdInst.Stdout = os.Stdout
	cmdInst.Stderr = os.Stderr

	// Run install and await
	if err = cmdInst.Run(); err != nil {
		log.Fatalf("Npm install: %v", err)
	}

	cmd := exec.Command("npm", "run", "dev")
	cmd.Dir = cmdDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run dev without awaiting
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

func wsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("received request %q\n", r.URL.Path)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("Websocket: %v", err)
	}

	cmd := exec.Command("bash")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("Pty: %v", err)
	}

	//// TODO: Close?
	// defer func() {
	// 	if closeErr := ptmx.Close(); closeErr != nil {
	// 		log.Fatalf("ptmx: %v", closeErr)
	// 	}
	// }()

	go ptyRead(ptmx, conn)
	go wsRead(conn, ptmx)
}

// ptyRead reads pty output and writes it to the websocket
func ptyRead(ptmx *os.File, conn *websocket.Conn) {
	log.Println("Reading pty.")

	b := make([]byte, bufferSize)

	for {
		n, err := ptmx.Read(b)
		if err != nil {
			log.Fatalf("read: %v", err)
		}

		// log.Printf("[%d] %q", n, b[:n])
		conn.WriteMessage(websocket.TextMessage, b[:n])
	}
}

// wsRead reads websocket input and writes it to the pty
func wsRead(conn *websocket.Conn, ptmx *os.File) {
	log.Println("Reading websocket.")

	for {
		msgType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Websocket read: %v\n", err)
			return
		}

		switch msgType {
		case websocket.TextMessage:
			// log.Printf("ws text: %v, %q", msgType, p)
			ptmx.Write(p)
		default:
			log.Printf("ws type: %v, data: %v\n", msgType, p)
		}
	}
}
