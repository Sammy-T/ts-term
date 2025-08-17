package websocket

import (
	"sync"

	"github.com/gorilla/websocket"
)

// SyncedWebsocket is a wrapper around *websocket.Conn
// which syncs the writes to the WebSocket with a Mutex.
type SyncedWebsocket struct {
	Conn *websocket.Conn
	Mu   *sync.Mutex
}

// WriteMessage is a helper method for getting a writer using NextWriter,
// writing the message and closing the writer.
func (s SyncedWebsocket) WriteMessage(messageType int, data []byte) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	return s.Conn.WriteMessage(messageType, data)
}

// ReadMessage is a helper method for getting a reader using NextReader
// and reading from that reader to a buffer.
func (s SyncedWebsocket) ReadMessage() (messageType int, p []byte, err error) {
	return s.Conn.ReadMessage()
}

// WriteJSON writes the JSON encoding of v as a message.
//
// See the documentation for encoding/json Marshal for details about the conversion of Go values to JSON.
func (s SyncedWebsocket) WriteJSON(v any) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	return s.Conn.WriteJSON(v)
}

// ReadJSON reads the next JSON-encoded message from the connection
// and stores it in the value pointed to by v.
//
// See the documentation for the encoding/json Unmarshal function for details
// about the conversion of JSON to a Go value.
func (s SyncedWebsocket) ReadJSON(v any) error {
	return s.Conn.ReadJSON(v)
}

// Close closes the underlying network connection without sending or waiting for a close message.
func (s SyncedWebsocket) Close() error {
	return s.Conn.Close()
}
