package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"

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

func tsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request %q\n", r.URL.Path)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("Websocket: %v", err)
	}

	//// TODO: Create single session server
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
	tsUpgrader := createUpgraderTs(server, client)

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

		go ptyToWs(ptmx, conn)
		go wsToPty(conn, ptmx)
	}

	return http.HandlerFunc(h)
}
