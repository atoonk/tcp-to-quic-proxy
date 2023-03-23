package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/atoonk/tcp-to-any-proxy/internal/tlsconfig"
	"github.com/quic-go/quic-go"
	"github.com/spf13/cobra"
)

func RunProxy(cmd *cobra.Command, args []string) {
	localAddr, _ := cmd.Flags().GetString("localAddr")
	remoteAddr, _ := cmd.Flags().GetString("remoteAddr")
	listenProto, _ := cmd.Flags().GetString("listenProto")
	remoteProto, _ := cmd.Flags().GetString("remoteProto")
	fmt.Printf("Listening on: %s %s\nProxying to: %s %s\n\n", listenProto, localAddr, remoteProto, remoteAddr)

	tlsConfig, err := tlsconfig.GenerateTLSConfig()
	if err != nil {
		log.Fatal(err)
	}

	quicConfig := &quic.Config{
		KeepAlivePeriod: time.Duration(10) * time.Second,
	}

	if listenProto == "tcp" {
		listener, err := net.Listen("tcp", localAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer listener.Close()

		for {
			localConn, err := listener.Accept()
			if err != nil {
				log.Println("Error accepting connection:", err)
				continue
			}
			if remoteProto == "ws" {
				go proxyTCPtoWS(localConn, remoteAddr)
			} else if remoteProto == "quic" {
				go proxyTCPtoQUIC(localConn, remoteAddr)
			} else {
				log.Fatal("Invalid remote protocol")
			}

		}
	}

	if listenProto == "quic" {
		listener, err := quic.ListenAddr(localAddr, tlsConfig, quicConfig)
		if err != nil {
			log.Fatal(err)
		}
		defer listener.Close()

		for {
			quicSession, err := listener.Accept(context.Background())
			if err != nil {
				log.Println("Error accepting QUIC session:", err)
				continue
			}

			go func() {
				quicStream, err := quicSession.AcceptStream(context.Background())
				if err != nil {
					log.Println("Error accepting QUIC stream:", err)
					return
				}
				if remoteProto == "ws" {
					proxyQUICtoWS(quicStream, remoteAddr)
				} else if remoteProto == "tcp" {
					proxyQUICtoTCP(quicStream, remoteAddr)
				} else {
					log.Fatal("Invalid remote protocol")
				}
			}()
		}
	}
	if listenProto == "ws" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			proxyWStoTCP(w, r, remoteAddr)
		})
		// Start the server
		log.Fatal(http.ListenAndServe(localAddr, nil))
	}
}

func transferData(a, b io.ReadWriteCloser) {
	done := make(chan bool)

	go func() {
		io.Copy(a, b)
		done <- true
	}()

	go func() {
		io.Copy(b, a)
		done <- true
	}()

	<-done
}
