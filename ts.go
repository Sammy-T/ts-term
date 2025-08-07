package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
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
func createUpgraderTs(client *local.Client) websocket.Upgrader {
	checkOrigin := func(r *http.Request) bool {
		if dev {
			return true
		}

		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		validOriginHosts := getTailnetAddresses(status)

		host := r.Host
		origin := r.Header.Get("Origin")
		reqUrl, err := url.Parse(origin)
		var oHost string
		if err == nil {
			// Reconstruct the request origin's host ignoring the port
			// since we're allowing any machine in the tailnet
			// to host the frontend.
			oHost = strings.Split(reqUrl.Host, ":")[0]
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

// getTailnetAddresses returns all the Tailscale domains and IP addresses
// on the Tailnet.
func getTailnetAddresses(status *ipnstate.Status) []string {
	domain := status.Self.DNSName
	shortDomain := strings.Split(domain, ".")[0]
	tsIps := status.TailscaleIPs

	addresses := []string{domain, shortDomain}

	for _, ip := range tsIps {
		addresses = append(addresses, ip.String())
	}

	for _, peerStatus := range status.Peer {
		domain = peerStatus.DNSName
		shortDomain = strings.Split(domain, ".")[0]
		tsIps = peerStatus.TailscaleIPs

		addresses = append(addresses, domain, shortDomain)

		for _, ip := range tsIps {
			addresses = append(addresses, ip.String())
		}
	}

	return addresses
}
