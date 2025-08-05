package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

// pollStatus polls the status of the TS server until the server is running
// and outputs relevant status info to the WebSocket.
func pollStatus(r *http.Request, server *tsnet.Server, client *local.Client, conn *websocket.Conn) {
	var authDelivered bool

	for i := 0; i < 500; i++ {
		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		switch status.BackendState {
		case "NeedsLogin":
			if authDelivered || status.AuthURL == "" {
				break
			}

			msg := fmt.Sprintf("Auth required. Go to: %v\r\n", status.AuthURL)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}

			authDelivered = true
		case "Running":
			tsIp4, tsIp6 := server.TailscaleIPs()
			hostname := status.Self.HostName

			msg := fmt.Sprintf("Tailscale machine %v at %v %v\r\n", hostname, tsIp4, tsIp6)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}

			conn.Close()
			return
		}

		time.Sleep(1 * time.Second)
	}
}

// createUpgraderTs creates a WebSocket Upgrader with a CheckOrigin function
// that verifies requests against the provided Tailscale server.
func createUpgraderTs(server *tsnet.Server, client *local.Client) websocket.Upgrader {
	checkOrigin := func(r *http.Request) bool {
		if dev {
			return true
		}

		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		tsIp4, tsIp6 := server.TailscaleIPs()
		hostname := status.Self.HostName

		validOriginHosts := []string{hostname, tsIp4.String(), tsIp6.String()}

		host := r.Host
		origin := r.Header.Get("Origin")
		reqUrl, err := url.Parse(origin)
		var oHost string
		if err == nil {
			oHost = reqUrl.Host
		}

		log.Printf("Check origin\nhost: %q\norigin: %q\noHost: %q\n", host, origin, oHost)

		return host == oHost || slices.Contains(validOriginHosts, oHost)
	}

	tsUpgrader := websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		CheckOrigin:     checkOrigin,
	}

	return tsUpgrader
}
