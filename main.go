package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	cnLog "github.com/sammy-t/ts-term/internal/log"
	ws "github.com/sammy-t/ts-term/internal/websocket"
	"golang.org/x/crypto/ssh"
	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

const bufferSize int = 1024

var upgrader = websocket.Upgrader{
	ReadBufferSize:  bufferSize,
	WriteBufferSize: bufferSize,
}

var dev bool

func init() {
	godotenv.Load()

	flag.BoolVar(&dev, "dev", false, "development mode")
	flag.Parse()
}

func main() {
	http.Handle("/", getWebHandler())
	http.HandleFunc("/ts", tsHandler)

	addr := os.Getenv("TS_TERM_ADDR")
	if addr == "" {
		addr = ":3000"
	}

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
	defer conn.Close()

	hostname := createHostName()

	dir, err := os.MkdirTemp("", "tsnet-"+hostname)
	if err != nil {
		log.Fatalf("mkdir: %v\n", err)
	}
	defer os.RemoveAll(dir)

	controlUrl := os.Getenv("TS_CONTROL_URL")

	server := &tsnet.Server{
		Hostname:   hostname,
		Dir:        dir,
		Ephemeral:  true,
		ControlURL: controlUrl,
	}
	defer server.Close()

	tsAddr := ":80"
	if strings.HasPrefix(r.Header["Origin"][0], "https:") {
		tsAddr = ":443"
	}

	listener, err := server.Listen("tcp", tsAddr)
	if err != nil {
		log.Printf("ts listener: %v\n", err)
		return
	}
	defer listener.Close()

	client, err := server.LocalClient()
	if err != nil {
		log.Printf("ts client: %v\n", err)
		return
	}

	if tsAddr == ":443" {
		log.Println("Enabling tsnet TLS. HTTPS Certificates must be enabled in the admin panel for this to work.")

		listener = tls.NewListener(listener, &tls.Config{
			GetCertificate: client.GetCertificate,
		})
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

	log.Printf("Running %v server\n", hostname)

	err = http.Serve(listener, getTsServerHandler(listener, server, client))
	log.Printf("%v server closed: %v", hostname, err)
}

func getTsServerHandler(listener net.Listener, server *tsnet.Server, client *local.Client) http.Handler {
	tsUpgrader := createUpgraderTs(client)

	h := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request %v %q\n", server.Hostname, r.URL.Path)

		wsConn, err := tsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket: %v", err)
			listener.Close()
			return
		}

		// Wrap the WebSocket in a sync helper
		// since both PTY 'read' and 'error' write to the WebSocket.
		conn := &ws.SyncedWebsocket{
			Conn: wsConn,
			Mu:   &sync.Mutex{},
		}

		cLog := cnLog.ConnLog{
			Conn:     conn,
			Listener: listener,
		}

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

		//// TODO: TEMP
		username := os.Getenv("SSH_USER")
		password := os.Getenv("SSH_PWD")
		addr := os.Getenv("SSH_ADDR")

		config := &ssh.ClientConfig{
			User: username,
			Auth: []ssh.AuthMethod{
				ssh.Password(password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		// Connect to the address through the tailnet
		tsConn, err := server.Dial(r.Context(), "tcp", addr)
		if err != nil {
			cLog.LessFatalf("ts dial: %v", err)
			return
		}

		// Create an SSH connection using the tailnet connection
		sshConn, newChan, reqs, err := ssh.NewClientConn(tsConn, addr, config)
		if err != nil {
			cLog.LessFatalf("sshConn: %v", err)
			return
		}

		sshClient := ssh.NewClient(sshConn, newChan, reqs)
		defer sshClient.Close()

		session, err := sshClient.NewSession()
		if err != nil {
			cLog.LessFatalf("sess: %v", err)
			return
		}
		defer session.Close()

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,     // enable echoing
			ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
		}

		// Request a PTY on the SSH session with an arbitrary height and width.
		// The frontend will send updated height and width once it's connected.
		if err = session.RequestPty("xterm-256color", 40, 80, modes); err != nil {
			cLog.LessFatalf("req pty: %v", err)
			return
		}

		onClosed := func() {
			listener.Close()
		}

		go ptyErrToWs(session, conn, onClosed)
		go ptyToWs(session, conn, onClosed)
		go wsToPty(conn, session, onClosed)

		if err = session.Shell(); err != nil {
			cLog.LessFatalf("shell: %v", err)
			return
		}

		// Wait for the remote command to exit.
		// This ensures the i/o pipes stay alive while we're using them.
		if err = session.Wait(); err != nil {
			cLog.LessFatalf("sess wait: %v", err)
			return
		}

		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

		if err = conn.WriteMessage(websocket.CloseMessage, closeMsg); err != nil {
			cLog.LessFatalf("ws close: %v", err)
			return
		}

		listener.Close()
	}

	return http.HandlerFunc(h)
}
