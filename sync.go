package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// ptyToWs reads PTY output and writes it to the WebSocket.
//
// NOTE: The WebSocket and PTY are closed when the PTY
// errors or closes.
func ptyToWs(ptmx *os.File, conn *websocket.Conn, onClosed func()) {
	log.Println("Reading pty.")

	defer func() {
		conn.Close()
		ptmx.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	b := make([]byte, bufferSize)

	for {
		n, err := ptmx.Read(b)
		if err != nil {
			log.Printf("read: %v", err)
			return
		}

		// log.Printf("[%d] %q", n, b[:n])
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
func wsToPty(conn *websocket.Conn, ptmx *os.File, onClosed func()) {
	log.Println("Reading websocket.")

	defer func() {
		conn.Close()
		ptmx.Close()

		if onClosed != nil {
			onClosed()
		}
	}()

	for {
		var msg wsMessage

		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Websocket read: %v\n", err)
			return
		}

		switch msg.Type {
		case "input":
			// log.Printf("ws text: %v, %q", msg.Type, msg.Data)
			ptmx.Write([]byte(msg.Data))
		case "size":
			log.Printf("size %v\n", msg.Data)

			var size pty.Winsize
			if err := json.Unmarshal([]byte(msg.Data), &size); err != nil {
				log.Printf("size: %v\n", err)
				break
			}

			if err := pty.Setsize(ptmx, &size); err != nil {
				log.Printf("set size: %v\n", err)
			}
		default:
			log.Printf("ws type: %v, data: %q\n", msg.Type, msg.Data)
		}
	}
}
