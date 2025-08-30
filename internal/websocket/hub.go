package websocket

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

type Hub struct {
	Conn      *SyncedWebsocket
	listeners map[MessageType][]chan msgResp
	mu        *sync.Mutex
}

type msgResp struct {
	msg Message
	err error
}

func NewHub(conn *SyncedWebsocket) Hub {
	h := Hub{
		Conn:      conn,
		listeners: make(map[MessageType][]chan msgResp),
		mu:        &sync.Mutex{},
	}

	go h.listen()

	return h
}

func (h Hub) listen() {
	readLimit := 60 * time.Second

	h.Conn.SetReadDeadline(time.Now().Add(readLimit))

	h.Conn.SetPongHandler(func(appData string) error {
		h.Conn.SetReadDeadline(time.Now().Add(readLimit))
		return nil
	})

	var err error

	for {
		if err = h.readMessages(); err != nil {
			log.Printf("closing hub listener: %v", err)
			return
		}
	}
}

func (h Hub) AwaitMsg(msgType MessageType, timeout time.Duration) (Message, error) {
	var msg Message
	var err error

	ch := h.registerListener(msgType)

	go func() {
		if timeout.Milliseconds() == 0 {
			return
		}

		time.Sleep(timeout)

		if msg.Type == msgType || err != nil {
			return
		}

		ch <- msgResp{err: fmt.Errorf("await msg %q timed out", msgType)}
	}()

	mResp := <-ch

	msg = mResp.msg
	err = mResp.err

	h.unregisterListener(msgType, ch)

	return msg, err
}

func (h Hub) readMessages() error {
	var msg Message

	if err := h.Conn.ReadJSON(&msg); err != nil {
		return err
	}

	log.Printf("hub msg: %q", msg.Type)

	switch msg.Type {
	case MessageError, MessageSshErr, MessageWsError:
		// Create an error response
		resp := msgResp{
			msg: msg,
			err: errors.New(string(msg.Type)),
		}

		// Notify all listeners
		for _, channels := range h.getAllListeners() {
			for _, respChan := range channels {
				respChan <- resp
			}
		}
		return nil
	}

	channels, ok := h.getListeners(msg.Type)
	if !ok {
		return nil
	}

	// Notify listeners registered for the message type
	for _, ch := range channels {
		ch <- msgResp{msg: msg, err: nil}
	}

	return nil
}

func (h Hub) registerListener(msgType MessageType) chan msgResp {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan msgResp)

	h.listeners[msgType] = append(h.listeners[msgType], ch)

	return ch
}

func (h Hub) unregisterListener(msgType MessageType, ch chan msgResp) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.listeners[msgType] = slices.DeleteFunc(h.listeners[msgType], func(channel chan msgResp) bool {
		return channel == ch
	})
}

func (h Hub) getAllListeners() map[MessageType][]chan msgResp {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.listeners
}

func (h Hub) getListeners(msgType MessageType) ([]chan msgResp, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	channels, ok := h.listeners[msgType]

	return channels, ok
}
