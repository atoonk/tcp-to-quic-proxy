package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

func proxyWStoTCP(w http.ResponseWriter, r *http.Request, remoteAddr string) {
	wsConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("failed to set websocket upgrade: %s", err)
		return
	}
	defer wsConn.Close(websocket.StatusNormalClosure, "")

	wsNetConn := NetConn(r.Context(), wsConn, websocket.MessageBinary)

	tcpConn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Println("Error dialing remote TCP address:", err)
		return
	}
	defer tcpConn.Close()

	transferData(wsNetConn, tcpConn)

	if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
		return
	}
	if err != nil {
		log.Printf("failed to proxy traffic from %s: %s", r.RemoteAddr, err)
		return
	}
}


func NetConn(ctx context.Context, c *websocket.Conn, msgType websocket.MessageType) net.Conn {
	nc := &netConn{
		c:       c,
		msgType: msgType,
	}

	var writeCancel context.CancelFunc
	nc.writeContext, writeCancel = context.WithCancel(ctx)
	nc.writeTimer = time.AfterFunc(math.MaxInt64, func() {
		nc.afterWriteDeadline.Store(true)
		if nc.writing.Load() {
			writeCancel()
		}
	})
	if !nc.writeTimer.Stop() {
		<-nc.writeTimer.C
	}

	var readCancel context.CancelFunc
	nc.readContext, readCancel = context.WithCancel(ctx)
	nc.readTimer = time.AfterFunc(math.MaxInt64, func() {
		nc.afterReadDeadline.Store(true)
		if nc.reading.Load() {
			readCancel()
		}
	})
	if !nc.readTimer.Stop() {
		<-nc.readTimer.C
	}

	return nc
}

type netConn struct {
	c       *websocket.Conn
	msgType websocket.MessageType

	writeTimer         *time.Timer
	writeContext       context.Context
	writing            atomic.Bool
	afterWriteDeadline atomic.Bool

	readTimer         *time.Timer
	readContext       context.Context
	reading           atomic.Bool
	afterReadDeadline atomic.Bool

	readMu sync.Mutex
	eofed  bool
	reader io.Reader
}

var _ net.Conn = &netConn{}

func (c *netConn) Close() error {
	return c.c.Close(websocket.StatusNormalClosure, "")
}

func (c *netConn) Write(p []byte) (int, error) {
	if c.afterWriteDeadline.Load() {
		return 0, os.ErrDeadlineExceeded
	}

	if swapped := c.writing.CompareAndSwap(false, true); !swapped {
		panic("Concurrent writes not allowed")
	}
	defer c.writing.Store(false)

	err := c.c.Write(c.writeContext, c.msgType, p)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (c *netConn) Read(p []byte) (int, error) {
	if c.afterReadDeadline.Load() {
		return 0, os.ErrDeadlineExceeded
	}

	c.readMu.Lock()
	defer c.readMu.Unlock()
	if swapped := c.reading.CompareAndSwap(false, true); !swapped {
		panic("Concurrent reads not allowed")
	}
	defer c.reading.Store(false)

	if c.eofed {
		return 0, io.EOF
	}

	if c.reader == nil {
		typ, r, err := c.c.Reader(c.readContext)
		if err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				c.eofed = true
				return 0, io.EOF
			}
			return 0, err
		}
		if typ != c.msgType {
			err := fmt.Errorf("unexpected frame type read (expected %v): %v", c.msgType, typ)
			c.c.Close(websocket.StatusUnsupportedData, err.Error())
			return 0, err
		}
		c.reader = r
	}

	n, err := c.reader.Read(p)
	if err == io.EOF {
		c.reader = nil
		err = nil
	}
	return n, err
}

type websocketAddr struct {
}

func (a websocketAddr) Network() string {
	return "websocket"
}

func (a websocketAddr) String() string {
	return "websocket/unknown-addr"
}

func (c *netConn) RemoteAddr() net.Addr {
	return websocketAddr{}
}

func (c *netConn) LocalAddr() net.Addr {
	return websocketAddr{}
}

func (c *netConn) SetDeadline(t time.Time) error {
	c.SetWriteDeadline(t)
	c.SetReadDeadline(t)
	return nil
}

func (c *netConn) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		c.writeTimer.Stop()
	} else {
		c.writeTimer.Reset(time.Until(t))
	}
	c.afterWriteDeadline.Store(false)
	return nil
}

func (c *netConn) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		c.readTimer.Stop()
	} else {
		c.readTimer.Reset(time.Until(t))
	}
	c.afterReadDeadline.Store(false)
	return nil
}
