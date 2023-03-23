package proxy

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/atoonk/tcp-to-any-proxy/internal/tlsconfig"
	"github.com/quic-go/quic-go"
	"nhooyr.io/websocket"
	// (...)
)

func proxyTCPtoQUIC(localConn net.Conn, remoteAddr string) {
	defer localConn.Close()

	tlsConfig, err := tlsconfig.GenerateTLSConfig()
	if err != nil {
		log.Println("Error generating TLS config:", err)
		return
	}

	quicConfig := &quic.Config{}

	quicSession, err := quic.DialAddr(remoteAddr, tlsConfig, quicConfig)
	if err != nil {
		log.Println("Error dialing remote QUIC address:", err)
		return
	}
	defer quicSession.CloseWithError(0, "")

	quicStream, err := quicSession.OpenStreamSync(context.Background())
	if err != nil {
		log.Println("Error opening QUIC stream:", err)
		return
	}
	defer quicStream.Close()

	transferData(localConn, quicStream)
}

func proxyTCPtoWS(localConn net.Conn, remoteAddr string) {
	defer localConn.Close()
	ctx := context.Background()

	fmt.Println("Dialing remote WebSocket address:", remoteAddr)
	wsConn, _, err := websocket.Dial(ctx, remoteAddr, nil)
	if err != nil {
		log.Println("Error dialing remote WebSocket address:", err)
		return
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "")
	wsNetConn := NetConn(ctx, wsConn, websocket.MessageBinary)

	transferData(localConn, wsNetConn)
}
