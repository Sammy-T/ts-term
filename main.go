package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

const bufferSize int = 1024

var upgrader = websocket.Upgrader{
	ReadBufferSize:  bufferSize,
	WriteBufferSize: bufferSize,
}

var dev bool

func main() {
	flag.BoolVar(&dev, "dev", false, "development mode")

	flag.Parse()

	http.Handle("/", getWebHandler())
	http.HandleFunc("/ts", tsHandler)

	log.Println("Starting Go server...")
	log.Fatal(http.ListenAndServe(":3000", nil))
}

func getWebHandler() http.Handler {
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

func tsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request %q\n", r.URL.Path)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("Websocket: %v", err)
	}

	server := &tsnet.Server{
		Ephemeral: true,
	}
	defer server.Close()

	listener, err := server.Listen("tcp", ":80")
	if err != nil {
		log.Fatalf("ts listener: %v", err)
	}
	defer listener.Close()

	client, err := server.LocalClient()
	if err != nil {
		log.Fatalf("ts client: %v", err)
	}

	go pollStatus(r, server, client, conn)

	log.Println("Starting TS server...")
	log.Fatal(http.Serve(listener, getTsServerHandler(server, client)))
}

func getTsServerHandler(server *tsnet.Server, client *local.Client) http.Handler {
	checkOrigin := func(r *http.Request) bool {
		if dev {
			return true
		}

		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		tsIp4, tsIp6 := server.TailscaleIPs()
		hostname := status.Self.HostName

		validOriginHosts := []string{hostname, tsIp4.String(), tsIp6.String()}

		host := r.Host
		origin := r.Header.Get("Origin")
		reqUrl, err := url.Parse(origin)
		var oHost string
		if err == nil {
			oHost = reqUrl.Host
		}

		log.Printf("Check origin\nhost: %q\norigin: %q\noHost: %q\n", host, origin, oHost)

		return host == oHost || slices.Contains(validOriginHosts, oHost)
	}

	tsUpgrader := websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		CheckOrigin:     checkOrigin,
	}

	h := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request %q\n", r.URL.Path)

		conn, err := tsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket: %v", err)
			return
		}

		status, err := client.Status(r.Context())
		if err != nil {
			log.Printf("ts status: %v", err)
			return
		}

		who, err := client.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			log.Printf("ts who: %v", err)
			return
		}

		msg := fmt.Sprintf("Connected to %v as %v from %v (%v).\r\n",
			status.Self.HostName,
			who.UserProfile.DisplayName,
			who.Node.ComputedName,
			r.RemoteAddr,
		)

		if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			log.Printf("ws write: %v", err)
			return
		}

		cmd := exec.Command("bash")

		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Printf("pty: %v", err)
			return
		}

		go ptyRead(ptmx, conn)
		go wsRead(conn, ptmx)
	}

	return http.HandlerFunc(h)
}

func pollStatus(r *http.Request, server *tsnet.Server, client *local.Client, conn *websocket.Conn) {
	var authDelivered bool

	for i := 0; i < 500; i++ {
		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		switch status.BackendState {
		case "NeedsLogin":
			if authDelivered || status.AuthURL == "" {
				break
			}

			msg := fmt.Sprintf("Auth required. Go to: %v\r\n", status.AuthURL)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}

			authDelivered = true
		case "Running":
			tsIp4, tsIp6 := server.TailscaleIPs()
			hostname := status.Self.HostName

			msg := fmt.Sprintf("Tailscale machine %v at %v %v\r\n", hostname, tsIp4, tsIp6)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}

			conn.Close()
			return
		}

		time.Sleep(1 * time.Second)
	}
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
		if err = conn.WriteMessage(websocket.TextMessage, b[:n]); err != nil {
			log.Fatalf("ws write: %v", err)
		}
	}
}

// wsRead reads websocket input and writes it to the pty
func wsRead(conn *websocket.Conn, ptmx *os.File) {
	log.Println("Reading websocket.")

	defer func() {
		log.Println("Closing ws and pty.")

		if err := conn.Close(); err != nil {
			log.Fatalf("ws close: %v", err)
		}

		if err := ptmx.Close(); err != nil {
			log.Fatalf("ptmx close: %v", err)
		}
	}()

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
