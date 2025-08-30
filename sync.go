package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	ws "github.com/sammy-t/ts-term/internal/websocket"
	"golang.org/x/crypto/ssh"
)

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
func ptyErrToWs(mu *sync.Mutex, session *ssh.Session, conn *ws.SyncedWebsocket, onClosed func()) {
	log.Println("Reading pty err...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	b := make([]byte, bufferSize)

	mu.Lock()
	errPipe, err := session.StderrPipe()
	mu.Unlock()
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

		msg := ws.Message{
			Type: ws.MessageOutput,
			Data: string(b[:n]),
		}

		// log.Printf("read err [%d] %q", n, b[:n])
		if err = conn.WriteJSON(msg); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

// ptyToWs reads PTY output and writes it to the WebSocket.
//
// NOTE: The WebSocket and PTY are closed when the PTY
// errors or closes.
func ptyToWs(mu *sync.Mutex, session *ssh.Session, conn *ws.SyncedWebsocket, onClosed func()) {
	log.Println("Reading pty...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	b := make([]byte, bufferSize)

	mu.Lock()
	outPipe, err := session.StdoutPipe()
	mu.Unlock()
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

		msg := ws.Message{
			Type: ws.MessageOutput,
			Data: string(b[:n]),
		}

		// log.Printf("read [%d] %q", n, b[:n])
		if err = conn.WriteJSON(msg); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

// wsToPty reads WebSocket input and writes it to the PTY.
//
// NOTE: The WebSocket and PTY are closed when the Websocket
// connection errors or closes.
func wsToPty(mu *sync.Mutex, session *ssh.Session, conn *ws.SyncedWebsocket, onClosed func()) {
	log.Println("Reading websocket...")

	defer func() {
		conn.Close()
		session.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	mu.Lock()
	inPipe, err := session.StdinPipe()
	mu.Unlock()
	if err != nil {
		log.Printf("sess in: %v", err)
		return
	}

	for {
		var msg ws.Message

		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Websocket read: %v", err)
			return
		}

		switch msg.Type {
		case ws.MessageInput:
			// log.Printf("ws text: %v, %q", msg.Type, msg.Data)
			if n, err := inPipe.Write([]byte(msg.Data)); err != nil {
				log.Printf("ws write: [%v] %v", n, err)
			}
		case ws.MessageSize:
			log.Printf("size %v", msg.Data)

			var size winSize
			if err := json.Unmarshal([]byte(msg.Data), &size); err != nil {
				log.Printf("size: %v", err)
				break
			}

			if err := session.WindowChange(size.Rows, size.Cols); err != nil {
				log.Printf("set size: %v", err)
			}
		default:
			log.Printf("ws type: %v, data: %q", msg.Type, msg.Data)
		}
	}
}
