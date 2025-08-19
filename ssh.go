package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	ws "github.com/sammy-t/ts-term/internal/websocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"tailscale.com/tsnet"
)

func getHostKeyCallback(conn *ws.SyncedWebsocket, knownHostsPath string) ssh.HostKeyCallback {
	cb := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hostKeyCb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return err
		}

		var keyErr *knownhosts.KeyError

		err = hostKeyCb(hostname, remote, key)
		if err != nil && errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			log.Printf("key unknown: %v", keyErr)

			wsMsg := ws.Message{
				Type: ws.MessageSshHost,
				Data: hostname,
			}

			// Notify the user
			if wsErr := conn.WriteJSON(wsMsg); wsErr != nil {
				return fmt.Errorf("ws write: %w", wsErr)
			}

			// Await a response
			respMsg, respErr := awaitConnectionMsg(conn, 0)
			if respErr != nil || respMsg.Type != ws.MessageSshHostAct {
				log.Printf("host await msg: %v", respErr)
				return errors.New("host await msg error")
			}

			if respMsg.Data == "yes" {
				hostLine := knownhosts.Line([]string{hostname}, key)

				file, fileErr := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if fileErr != nil {
					log.Printf("file open: %v", fileErr)
					return fileErr
				}
				defer file.Close()

				if _, fileErr := file.WriteString(hostLine + "\n"); fileErr != nil {
					return fileErr
				}

				hostKeyCb, err = knownhosts.New(knownHostsPath)
				if err != nil {
					return err
				}

				// Retry with the updated known_hosts
				err = hostKeyCb(hostname, remote, key)
			}
		}

		return err
	}

	return cb
}

func reattemptSSH(r *http.Request, server *tsnet.Server, conn *ws.SyncedWebsocket, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	var sshErr error

	for range 5 {
		log.Println("Reattempting ssh...")

		msg := ws.Message{
			Type: ws.MessageSshErr,
		}

		if err := conn.WriteJSON(msg); err != nil {
			return nil, nil, nil, fmt.Errorf("json msg: %w", err)
		}

		respMsg, err := awaitConnectionMsg(conn, 0)
		if err != nil || respMsg.Type != ws.MessageSshCfg {
			if sshErr != nil {
				log.Printf("await msg: %v", err)
				return nil, nil, nil, sshErr
			}
			return nil, nil, nil, err
		}

		sshCfg := parseSshConfig(respMsg.Data)

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
			log.Printf("ssh reattempt: %v", err)
			sshErr = err
			continue
		}

		return sshConn, newChan, reqs, err
	}

	return nil, nil, nil, errors.New("max ssh attempts reached")
}

func parseSshConfig(resp string) map[string]string {
	// username:password:address:port
	parsed := strings.Split(resp, ":")

	return map[string]string{
		"username": parsed[0],
		"password": parsed[1],
		"address":  parsed[2] + ":" + parsed[3],
	}
}

func getKnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return path.Join(home, ".ssh", "known_hosts"), nil
}
