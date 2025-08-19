package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

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

	log.Printf("Serving ts-term on %v", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func getWebHandler() http.Handler {
	if dev {
		startDevServer("pnpm")
		return createDevHandler()
	}

	return http.FileServer(http.Dir("web/dist"))
}

func tsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request %q", r.URL.Path)

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("Websocket: %v", err)
	}

	conn := &ws.SyncedWebsocket{
		Conn: wsConn,
		Mu:   &sync.Mutex{},
	}
	defer conn.Close()

	hub := ws.NewHub(conn)

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

	log.Printf("Creating tsnet server %q...", hostname)
	listener, err := server.Listen("tcp", tsAddr)
	if err != nil {
		log.Printf("ts listener: %v", err)
		return
	}
	defer listener.Close()

	log.Println("Getting local client...")
	client, err := server.LocalClient()
	if err != nil {
		log.Printf("ts client: %v", err)
		return
	}

	if tsAddr == ":443" {
		log.Println("Enabling tsnet TLS. HTTPS Certificates must be enabled in the admin panel for this to work.")

		listener = tls.NewListener(listener, &tls.Config{
			GetCertificate: client.GetCertificate,
		})
	}

	go func() {
		log.Println("Starting ping...")

		for {
			time.Sleep(3 * time.Second)

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("ping err: %v", err)
				return
			}
		}
	}()

	log.Println("Polling status...")
	if err := pollStatus(r, server, client, conn); err != nil {
		log.Printf("poll status: %v", err)
		return
	}

	peerInfos, err := getPeerConnInfo(r, client)
	if err != nil {
		log.Printf("peer info: %v", err)
		return
	}

	infoBytes, err := json.Marshal(peerInfos)
	if err != nil {
		log.Printf("peer marshal: %v", err)
		return
	}

	wsMsg := ws.Message{
		Type: ws.MessagePeers,
		Data: string(infoBytes),
	}

	if err := conn.WriteJSON(wsMsg); err != nil {
		log.Printf("ws write peers: %v", err)
		return
	}

	// Await the ssh config info
	respMsg, err := hub.AwaitMsg(ws.MessageSshCfg, 0)
	if err != nil {
		log.Printf("%v await ssh cfg: %v", hostname, err)
		return
	}

	sshCfg := parseSshConfig(respMsg.Data)

	go func() {
		defer conn.Close()

		// Await the ts-websocket-opened message
		_, err := hub.AwaitMsg(ws.MessageWsOpened, 30*time.Second)
		if err != nil {
			log.Printf("%v websocket await: %v", hostname, err)
			listener.Close()
			return
		}

		log.Printf("%v websocket connected to client.", hostname)
	}()

	log.Printf("Running %v server", hostname)

	err = http.Serve(listener, getTsServerHandler(listener, server, client, sshCfg))
	log.Printf("%v server closed: %v", hostname, err)
}

func getTsServerHandler(listener net.Listener, server *tsnet.Server, client *local.Client, sshCfg map[string]string) http.Handler {
	tsUpgrader := createUpgraderTs(client)

	h := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request %v %q", server.Hostname, r.URL.Path)

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

		msg := fmt.Sprintf("Connected to %v as %v from %v (%v).",
			status.Self.HostName,
			who.UserProfile.DisplayName,
			who.Node.ComputedName,
			r.RemoteAddr,
		)

		wsMsg := ws.Message{
			Type: ws.MessageInfo,
			Data: msg,
		}

		if err = conn.WriteJSON(wsMsg); err != nil {
			cLog.LessFatalf("ws write: %v", err)
			return
		}

		knownHostsPath := os.Getenv("TS_TERM_KNOWN_HOSTS")
		if knownHostsPath == "" {
			knownHostsPath, err = getKnownHostsPath()
			if err != nil {
				cLog.LessFatalf("known hosts path: %v", err)
				return
			}
		}

		hostKeyCb := getHostKeyCallback(conn, knownHostsPath)

		config := &ssh.ClientConfig{
			User: sshCfg["username"],
			Auth: []ssh.AuthMethod{
				ssh.Password(sshCfg["password"]),
			},
			HostKeyCallback: hostKeyCb,
		}

		// Connect to the address through the tailnet
		tsConn, err := server.Dial(r.Context(), "tcp", sshCfg["address"])
		if err != nil {
			cLog.LessFatalf("ts dial: %v", err)
			return
		}

		// Create an SSH connection using the tailnet connection
		sshConn, newChan, reqs, err := ssh.NewClientConn(tsConn, sshCfg["address"], config)
		if err != nil {
			cLog.Printf("ssh conn: %v", err)
			sshConn, newChan, reqs, err = reattemptSSH(r, server, conn, config)
		}
		// Return if reattempts fail
		if err != nil {
			log.Printf("ssh conn: %v", err)
			cLog.LessFatalf("ssh failed")
			return
		}

		wsMsg = ws.Message{
			Type: ws.MessageSshSuccess,
		}

		if err = conn.WriteJSON(wsMsg); err != nil {
			cLog.LessFatalf("ws write: %v", err)
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

// awaitConnectionMsg awaits the next notification on the provided Websocket connection
// and returns the parsed response or an error.
//
// The connection is closed if a response isn't received before the timeout.
func awaitConnectionMsg(conn *ws.SyncedWebsocket, timeout time.Duration) (ws.Message, error) {
	var msg ws.Message
	var err error

	go func() {
		if timeout.Milliseconds() == 0 {
			return
		}

		time.Sleep(timeout)

		if msg != (ws.Message{}) || err != nil {
			return
		}

		msg := "websocket timed out."

		log.Println(msg)

		wsMsg := websocket.FormatCloseMessage(websocket.CloseGoingAway, msg)
		conn.WriteMessage(websocket.CloseMessage, wsMsg)
	}()

	if err := conn.ReadJSON(&msg); err != nil {
		return msg, fmt.Errorf("read err: %w", err)
	}

	switch msg.Type {
	case ws.MessageSshCfg, ws.MessageSshHost, ws.MessageWsOpened:
		return msg, nil
	case ws.MessageWsError:
		return msg, errors.New("websocket errored")
	default:
		return msg, fmt.Errorf("invalid msg received. [%v] %q", msg.Type, msg.Data)
	}
}
