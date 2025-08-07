package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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

	addr := ":3000"

	log.Printf("Serving ts-term on %v\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
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

	hostname := createHostName()

	dir, err := os.MkdirTemp("", "tsnet-"+hostname)
	if err != nil {
		log.Fatalf("mkdir: %v\n", err)
	}
	defer os.RemoveAll(dir)

	server := &tsnet.Server{
		Hostname:  hostname,
		Dir:       dir,
		Ephemeral: true,
	}
	defer server.Close()

	listener, err := server.Listen("tcp", ":80")
	if err != nil {
		log.Fatalf("ts listener: %v", err)
	}

	client, err := server.LocalClient()
	if err != nil {
		log.Fatalf("ts client: %v", err)
	}

	go func() {
		defer conn.Close()

		if err := pollStatus(r, server, client, conn); err != nil {
			log.Printf("poll status: %v\n", err)
			listener.Close()
			return
		}

		if err := awaitTsWsConnection(conn); err != nil {
			log.Printf("%v conn await: %v\n", hostname, err)
			listener.Close()
			return
		}

		log.Printf("%v websocket connected to client.\n", hostname)
	}()

	log.Printf("Serving %v server\n", hostname)
	log.Printf("%v server: %v", hostname, http.Serve(listener, getTsServerHandler(hostname, listener, client)))
}

func getTsServerHandler(hostname string, listener net.Listener, client *local.Client) http.Handler {
	tsUpgrader := createUpgraderTs(client)

	h := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request %v %q\n", hostname, r.URL.Path)

		conn, err := tsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket: %v", err)
			listener.Close()
			return
		}

		cLog := connLog{conn, listener}

		status, err := client.Status(r.Context())
		if err != nil {
			cLog.LessFatalf("ts status: %v", err)
			return
		}

		who, err := client.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			cLog.LessFatalf("ts who: %v", err)
			return
		}

		msg := fmt.Sprintf("Connected to %v as %v from %v (%v).\r\n",
			status.Self.HostName,
			who.UserProfile.DisplayName,
			who.Node.ComputedName,
			r.RemoteAddr,
		)

		if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			cLog.LessFatalf("ws write: %v", err)
			return
		}

		cmd := exec.Command("bash")

		ptmx, err := pty.Start(cmd)
		if err != nil {
			cLog.LessFatalf("pty: %v", err)
			return
		}

		onClosed := func() {
			listener.Close()
		}

		go ptyToWs(ptmx, conn, onClosed)
		go wsToPty(conn, ptmx, onClosed)
	}

	return http.HandlerFunc(h)
}
