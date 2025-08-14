package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
	ws "github.com/sammy-t/ts-term/internal/websocket"
	"golang.org/x/crypto/ssh"
)

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type winSize struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
	X    int `json:"x"`
	Y    int `json:"y"`
}

const ioDelay time.Duration = 10 * time.Millisecond

// ptyToWs reads PTY error output and writes it to the WebSocket.
//
// NOTE: The WebSocket and PTY are closed when the PTY
// errors or closes.
func ptyErrToWs(session *ssh.Session, conn *ws.SyncedWebsocket, onClosed func()) {
	log.Println("Reading pty err...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	b := make([]byte, bufferSize)

	errPipe, err := session.StderrPipe()
	if err != nil {
		log.Printf("sess err: %v", err)
		return
	}

	for {
		time.Sleep(ioDelay)

		n, err := errPipe.Read(b)
		if err != nil && n > 0 {
			log.Printf("read err: %v", err)
			return
		}

		if n == 0 {
			continue
		}

		// log.Printf("read err [%d] %q", n, b[:n])
		if err = conn.WriteMessage(websocket.TextMessage, b[:n]); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

// ptyToWs reads PTY output and writes it to the WebSocket.
//
// NOTE: The WebSocket and PTY are closed when the PTY
// errors or closes.
func ptyToWs(session *ssh.Session, conn *ws.SyncedWebsocket, onClosed func()) {
	log.Println("Reading pty...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	b := make([]byte, bufferSize)

	outPipe, err := session.StdoutPipe()
	if err != nil {
		log.Printf("sess out: %v", err)
		return
	}

	for {
		time.Sleep(ioDelay)

		n, err := outPipe.Read(b)
		if err != nil && n > 0 {
			log.Printf("read: %v", err)
			return
		}

		if n == 0 {
			continue
		}

		// log.Printf("read [%d] %q", n, b[:n])
		if err = conn.WriteMessage(websocket.TextMessage, b[:n]); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

// wsToPty reads WebSocket input and writes it to the PTY.
//
// NOTE: The WebSocket and PTY are closed when the Websocket
// connection errors or closes.
func wsToPty(conn *ws.SyncedWebsocket, session *ssh.Session, onClosed func()) {
	log.Println("Reading websocket...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	inPipe, err := session.StdinPipe()
	if err != nil {
		log.Printf("sess in: %v", err)
		return
	}

	for {
		var msg wsMessage

		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Websocket read: %v\n", err)
			return
		}

		switch msg.Type {
		case "input":
			// log.Printf("ws text: %v, %q", msg.Type, msg.Data)
			if n, err := inPipe.Write([]byte(msg.Data)); err != nil {
				log.Printf("ws write: [%v] %v", n, err)
			}
		case "size":
			log.Printf("size %v\n", msg.Data)

			var size winSize
			if err := json.Unmarshal([]byte(msg.Data), &size); err != nil {
				log.Printf("size: %v\n", err)
				break
			}

			if err := session.WindowChange(size.Rows, size.Cols); err != nil {
				log.Printf("set size: %v\n", err)
			}
		default:
			log.Printf("ws type: %v, data: %q\n", msg.Type, msg.Data)
		}
	}
}
