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
	"golang.org/x/crypto/ssh/knownhosts"
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
		startDevServer("pnpm")
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

	log.Printf("Creating tsnet server %q...\n", hostname)
	listener, err := server.Listen("tcp", tsAddr)
	if err != nil {
		log.Printf("ts listener: %v\n", err)
		return
	}
	defer listener.Close()

	log.Println("Getting local client...")
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

	log.Println("Polling status...")
	if err := pollStatus(r, server, client, conn); err != nil {
		log.Printf("poll status: %v\n", err)
		return
	}

	peerInfos, err := getPeerConnInfo(r, client)
	if err != nil {
		log.Printf("peer info: %v\n", err)
		return
	}

	infoBytes, err := json.Marshal(peerInfos)
	if err != nil {
		log.Printf("peer marshal: %v\n", err)
		return
	}

	wsMsg := ws.Message{
		Type: ws.MessagePeers,
		Data: string(infoBytes),
	}

	if err := conn.WriteJSON(wsMsg); err != nil {
		log.Printf("ws write peers: %v\n", err)
		return
	}

	// Await the ssh config info
	resp, err := awaitConnectionMsg(conn, 0)
	if err != nil || resp[0] != "ssh-config" {
		log.Printf("%v conn await %v: %v\n", hostname, resp, err)
		return
	}

	sshCfg := parseSshConfig(resp)

	go func() {
		defer conn.Close()

		// Await the ts-websocket-opened message
		resp, err := awaitConnectionMsg(conn, 30*time.Second)
		if err != nil || resp[0] != "ts-websocket-opened" {
			log.Printf("%v conn await %v: %v\n", hostname, resp, err)
			listener.Close()
			return
		}

		log.Printf("%v websocket connected to client.\n", hostname)
	}()

	log.Printf("Running %v server\n", hostname)

	err = http.Serve(listener, getTsServerHandler(listener, server, client, sshCfg))
	log.Printf("%v server closed: %v", hostname, err)
}

func getTsServerHandler(listener net.Listener, server *tsnet.Server, client *local.Client, sshCfg map[string]string) http.Handler {
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

		hostKeyCb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			cLog.LessFatalf("known hosts cb: %v", err)
			return
		}

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
			cLog.Printf("sshConn: %v", err)
			sshConn, newChan, reqs, err = reattemptSSH(r, server, conn, config)
		}
		// Return if reattempts fail
		if err != nil {
			cLog.LessFatalf("sshConn: %v", err)
			return
		}

		wsMsg = ws.Message{
			Type: ws.MessageStatus,
			Data: "ssh-success",
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
func awaitConnectionMsg(conn *websocket.Conn, timeout time.Duration) ([]string, error) {
	var parsed []string
	var err error

	go func() {
		if timeout.Milliseconds() == 0 {
			return
		}

		time.Sleep(timeout)

		if len(parsed) > 0 || err != nil {
			return
		}

		msg := "websocket timed out."

		log.Println(msg)

		wsMsg := websocket.FormatCloseMessage(websocket.CloseGoingAway, msg)
		conn.WriteMessage(websocket.CloseMessage, wsMsg)
	}()

	var msg ws.Message

	if err := conn.ReadJSON(&msg); err != nil {
		return parsed, fmt.Errorf("read err: %v", err)
	}

	if msg.Type != ws.MessageStatus {
		return parsed, fmt.Errorf("invalid msg received. [%v] %q", msg.Type, msg.Data)
	}

	parsed = strings.Split(string(msg.Data), ":")

	switch parsed[0] {
	case ws.StatusSshCfg, ws.StatusWsOpened:
		return parsed, nil
	case ws.StatusWsError:
		return parsed, errors.New("websocket errored")
	default:
		return parsed, fmt.Errorf("invalid msg received. [%v] %q", msg.Type, msg.Data)
	}
}
