package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"

	ws "github.com/sammy-t/ts-term/internal/websocket"
	"golang.org/x/crypto/ssh"
	"tailscale.com/tsnet"
)

func reattemptSSH(r *http.Request, server *tsnet.Server, conn *ws.SyncedWebsocket, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	var sshErr error

	for i := 0; i < 5; i++ {
		log.Println("Reattempting ssh...")

		msg := ws.Message{
			Type: ws.MessageStatus,
			Data: "ssh-error",
		}

		if err := conn.WriteJSON(msg); err != nil {
			return nil, nil, nil, fmt.Errorf("json msg: %w", err)
		}

		resp, err := awaitConnectionMsg(conn.Conn, 0)
		if err != nil || resp[0] != "ssh-config" {
			if sshErr != nil {
				return nil, nil, nil, sshErr
			}
			return nil, nil, nil, err
		}

		sshCfg := parseSshConfig(resp)

		config.User = sshCfg["username"]
		config.Auth = []ssh.AuthMethod{
			ssh.Password(sshCfg["password"]),
		}

		tsConn, err := server.Dial(r.Context(), "tcp", sshCfg["address"])
		if err != nil {
			return nil, nil, nil, fmt.Errorf("ts dial: %w", err)
		}

		sshConn, newChan, reqs, err := ssh.NewClientConn(tsConn, sshCfg["address"], config)
		if err != nil {
			log.Printf("ssh reattempt: %v\n", err)
			sshErr = err
			continue
		}

		return sshConn, newChan, reqs, err
	}

	return nil, nil, nil, errors.New("max ssh attempts reached")
}

func parseSshConfig(resp []string) map[string]string {
	// ssh-config:username:password:address:port
	return map[string]string{
		"username": resp[1],
		"password": resp[2],
		"address":  resp[3] + ":" + resp[4],
	}
}

func getKnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return path.Join(home, ".ssh", "known_hosts"), nil
}
