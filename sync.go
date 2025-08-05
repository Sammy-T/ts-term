package main

import (
	"log"
	"os"

	"github.com/gorilla/websocket"
)

// ptyToWs reads PTY output and writes it to the WebSocket.
func ptyToWs(ptmx *os.File, conn *websocket.Conn) {
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

// wsToPty reads WebSocket input and writes it to the PTY.
//
// NOTE: The WebSocket and PTY are closed when the Websocket
// connection errors or closes.
func wsToPty(conn *websocket.Conn, ptmx *os.File) {
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
