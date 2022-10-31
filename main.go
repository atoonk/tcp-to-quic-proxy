package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"

	quic "github.com/lucas-clemente/quic-go"
)

var localAddr *string = flag.String("l", "localhost:9999", "local address")
var remoteAddr *string = flag.String("r", "127.0.0.1:5201", "remote address")
var listenProto *string = flag.String("p", "tcp", "listen protocol")
var remoteProto *string = flag.String("u", "tcp", "remote protocol")

func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"proto"},
	}, nil
}

func main() {
	flag.Parse()
	fmt.Printf("Listening: %s %s  %v\nProxying: %v\n\n", *listenProto, *remoteProto, *localAddr, *remoteAddr)

	if *listenProto == "tcp" {
		listener, err := net.Listen("tcp", *localAddr)
		if err != nil {
			panic(err)
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			log.Println("New connection", conn.RemoteAddr())
			if err != nil {
				log.Println("error accepting connection", err)
				continue
			}

			go func() {
				defer conn.Close()
				if *remoteProto == "tcp" {
					conn2 := getTCPUpstream()
					if conn2 == nil {
						return
					}
					defer conn2.Close()
					fmt.Println("[+] upstream opened")

					closer := make(chan struct{}, 2)
					go copy(closer, conn2, conn)
					go copy(closer, conn, conn2)
					<-closer
				} else if *remoteProto == "quic" {
					conn2 := getQuicUpstream()
					if conn2 == nil {
						return
					}
					defer conn2.CloseWithError(0, "")
					stream, err := conn2.OpenStreamSync(context.Background())
					if err != nil {
						fmt.Println("[!] Error opening stream")
						return
					}
					defer stream.Close()

					closer := make(chan struct{}, 2)
					go copy(closer, stream, conn)
					go copy(closer, conn, stream)
					<-closer
				}
				log.Println("Connection complete", conn.RemoteAddr())
			}()
		}
	} else if *listenProto == "quic" {

		// Set up our TLS
		//tlsConfig, err := configureTLS()
		tlsConfig, err := generateTLSConfig()
		if err != nil {
			fmt.Println("[!] Error grabbing TLS certs")
			return
		}
		quicListener, err := quic.ListenAddr(*localAddr, tlsConfig, nil)

		if err != nil {
			fmt.Println("[!] Error binding to UDP/443")
			return
		}
		fmt.Println("[+] Listening on UDP/" + *localAddr + " for QUIC connections")

		for {
			fmt.Println("[+] Waiting for connection...")
			session, err := quicListener.Accept(context.Background())

			if err != nil {
				fmt.Println("[!] Error accepting connection from client")
				continue
			}
			go func() {

				fmt.Printf("[*] Accepted connection from %s\n", session.RemoteAddr().String())

				stream, err := session.AcceptStream(context.Background())

				if err != nil {
					fmt.Println("[!] Error accepting stream from QUIC client")
				}

				defer session.CloseWithError(0, "Bye")
				if err != nil {
					fmt.Println("[!] Error accepting stream")
					return
				}
				defer stream.Close()

				// Upstream connection
				if *remoteProto == "tcp" {
					conn2 := getTCPUpstream()
					if conn2 == nil {
						return
					}
					defer conn2.Close()
					fmt.Println("[+] upstream opened")

					closer := make(chan struct{}, 2)
					go copy(closer, conn2, stream)
					go copy(closer, stream, conn2)
					<-closer
				} else if *remoteProto == "quic" {
					conn2 := getQuicUpstream()
					if conn2 == nil {
						return
					}
					defer conn2.CloseWithError(0, "")
					//stream2, err := conn2.OpenStreamSync(context.Background())
					stream2, err := conn2.OpenStream()
					if err != nil {
						fmt.Println("[!] Error opening stream")
						return
					}
					defer stream.Close()

					closer := make(chan struct{}, 2)
					go copy(closer, stream, stream2)
					go copy(closer, stream2, stream)
					<-closer

				}
				log.Println("Connection complete", session.RemoteAddr())
			}()
		}
	} else {
		fmt.Println("[!] Invalid protocol specified")
		return
	}
}

func getQuicUpstream() (conn quic.Connection) {
	// Set up our TLS
	//tlsConfig, err := configureTLS()

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"proto"},
	}
	conn, err := quic.DialAddr(*remoteAddr, tlsConf, nil)
	if err != nil {
		panic(err)
	}
	return conn

}

func getTCPUpstream() (conn net.Conn) {
	conn, err := net.Dial("tcp", *remoteAddr)
	if err != nil {
		log.Println("error dialing remote addr", err)
		return nil
	}
	return conn
}

func copy(closer chan struct{}, dst io.Writer, src io.Reader) {
	//r := io.TeeReader(src, dst)
	//_, _ = io.Copy(os.Stdout, r)
	fmt.Println("[+] Copying data!")
	io.Copy(dst, src)
	fmt.Println("[+] done Copying data")
	closer <- struct{}{} // connection is closed, send signal to stop proxy
}
