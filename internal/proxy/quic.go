package proxy

import (
	"fmt"
	"log"
	"net"

	"github.com/quic-go/quic-go"
	"nhooyr.io/websocket"
)

func proxyQUICtoTCP(quicStream quic.Stream, remoteAddr string) {
	defer quicStream.Close()

	tcpConn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Println("Error dialing remote TCP address:", err)
		return
	}
	defer tcpConn.Close()

	transferData(quicStream, tcpConn)
}

func proxyQUICtoWS(quicStream quic.Stream, remoteAddr string) {
	defer quicStream.Close()

	fmt.Println("Dialing remote WebSocket address:", remoteAddr)
	wsConn, _, err := websocket.Dial(quicStream.Context(), remoteAddr, nil)
	if err != nil {
		log.Println("Error dialing remote WebSocket address:", err)
		return
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "")
	wsNetConn := NetConn(quicStream.Context(), wsConn, websocket.MessageBinary)

	transferData(quicStream, wsNetConn)
}
