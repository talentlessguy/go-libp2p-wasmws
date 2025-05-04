package wasmws

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

type websocketNetConn struct {
	conn      *websocket.Conn
	laddr     ma.Multiaddr
	raddr     ma.Multiaddr
	readBuf   []byte
	readPos   int
	readLock  sync.Mutex
	writeLock sync.Mutex

	// Track connection lifetime context
	closeCtx    context.Context
	cancelClose context.CancelFunc
}

var _ net.Conn = (*websocketNetConn)(nil)

func (w *websocketNetConn) Read(p []byte) (int, error) {
	w.readLock.Lock()
	defer w.readLock.Unlock()

	// Serve buffered data first
	if w.readPos < len(w.readBuf) {
		n := copy(p, w.readBuf[w.readPos:])
		w.readPos += n
		return n, nil
	}

	// Read full WebSocket message (blocking, but tied to connection lifetime)
	msgType, data, err := w.conn.Read(w.closeCtx)
	if err != nil {
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return 0, io.EOF
		}
		return 0, fmt.Errorf("websocket read failed: %w", err)
	}
	if msgType != websocket.MessageBinary {
		return 0, fmt.Errorf("expected binary message, got %d", msgType)
	}

	// Reset buffer
	w.readBuf = data
	w.readPos = 0

	n := copy(p, w.readBuf)
	w.readPos += n
	return n, nil
}

func (w *websocketNetConn) Write(p []byte) (int, error) {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()

	// Write entire payload as single WebSocket message
	err := w.conn.Write(w.closeCtx, websocket.MessageBinary, p)
	if err != nil {
		return 0, fmt.Errorf("websocket write failed: %w", err)
	}
	return len(p), nil
}

func (w *websocketNetConn) Close() error {
	w.cancelClose() // Cancel ongoing reads/writes
	return w.conn.Close(websocket.StatusNormalClosure, "closing")
}

func (w *websocketNetConn) LocalMultiaddr() ma.Multiaddr {
	return w.laddr
}

func (w *websocketNetConn) RemoteMultiaddr() ma.Multiaddr {
	return w.raddr
}

func (w *websocketNetConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1634}
}

func (w *websocketNetConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1634}
}

// WASM websockets don’t support deadlines, just stub
func (w *websocketNetConn) SetDeadline(t time.Time) error      { return nil }
func (w *websocketNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *websocketNetConn) SetWriteDeadline(t time.Time) error { return nil }

func newConn(raw *websocket.Conn, scope network.ConnManagementScope, raddr ma.Multiaddr) *websocketNetConn {
	laddr, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/0/ws")
	ctx, cancel := context.WithCancel(context.Background())

	return &websocketNetConn{
		conn:        raw,
		raddr:       raddr,
		laddr:       laddr,
		closeCtx:    ctx,
		cancelClose: cancel,
		readBuf:     make([]byte, 0),
	}
}
