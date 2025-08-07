package main

import (
	"log"
	"os"

	"github.com/gorilla/websocket"
)

// ptyToWs reads PTY output and writes it to the WebSocket.
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
