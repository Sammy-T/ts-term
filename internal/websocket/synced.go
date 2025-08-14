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

func (s SyncedWebsocket) WriteMessage(messageType int, data []byte) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	return s.Conn.WriteMessage(messageType, data)
}

func (s SyncedWebsocket) ReadMessage() (messageType int, p []byte, err error) {
	return s.Conn.ReadMessage()
}

func (s SyncedWebsocket) ReadJSON(v any) error {
	return s.Conn.ReadJSON(v)
}

func (s SyncedWebsocket) Close() error {
	return s.Conn.Close()
}
