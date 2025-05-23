package wasmws

import (
	"context"
	"fmt"

	ws "github.com/coder/websocket"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"

	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/multiformats/go-multiaddr-fmt"
	manet "github.com/multiformats/go-multiaddr/net"
)

var (
	wssComponent, _ = ma.NewComponent("wss", "")
	tlsComponent, _ = ma.NewComponent("tls", "")
	wsComponent, _  = ma.NewComponent("ws", "")
	tlsWsAddr       = ma.Multiaddr{*tlsComponent, *wsComponent}
)

var dialMatcher = mafmt.And(
	mafmt.Or(mafmt.IP, mafmt.DNS),
	mafmt.Base(ma.P_TCP),
	mafmt.Or(
		mafmt.Base(ma.P_WS),
		mafmt.And(
			mafmt.Or(
				mafmt.And(
					mafmt.Base(ma.P_TLS),
					mafmt.Base(ma.P_SNI)),
				mafmt.Base(ma.P_TLS),
			),
			mafmt.Base(ma.P_WS)),
		mafmt.Base(ma.P_WSS)))

type WebsocketTransport struct {
	transport.Transport
	upgrader transport.Upgrader
	rcmgr    network.ResourceManager
}

func (t *WebsocketTransport) CanDial(a ma.Multiaddr) bool {
	return dialMatcher.Matches(a)
}

func (t *WebsocketTransport) Protocols() []int {
	return []int{ma.P_WS, ma.P_WSS}
}

type capableConn struct {
	transport.CapableConn
}

func (t *WebsocketTransport) dialWithScope(ctx context.Context, raddr ma.Multiaddr, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	macon, err := t.maDial(ctx, raddr, connScope)
	if err != nil {
		return nil, err
	}
	conn, err := t.upgrader.Upgrade(ctx, t, macon, network.DirOutbound, p, connScope)
	if err != nil {
		return nil, fmt.Errorf("failed to upgrade: %w", err)
	}
	return &capableConn{CapableConn: conn}, nil
}

func (t *WebsocketTransport) maDial(ctx context.Context, raddr ma.Multiaddr, scope network.ConnManagementScope) (manet.Conn, error) {
	wsurl, err := parseMultiaddr(raddr)
	if err != nil {
		scope.Done()
		return nil, fmt.Errorf("parse multiaddr: %w", err)
	}

	wscon, _, err := ws.Dial(ctx, wsurl.String(), &ws.DialOptions{})
	if err != nil {
		scope.Done()
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}

	mnc, err := manet.WrapNetConn(newConn(wscon, scope, raddr))
	if err != nil {
		wscon.Close(ws.StatusInternalError, "wrap failed")
		scope.Done()
		return nil, err
	}
	return mnc, nil
}

func New(u transport.Upgrader, rcmgr network.ResourceManager) (*WebsocketTransport, error) {
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}

	// Create the transport
	t := &WebsocketTransport{
		upgrader: u,
		rcmgr:    rcmgr,
	}

	return t, nil
}

func (t *WebsocketTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection scope: %w", err)
	}
	c, err := t.dialWithScope(ctx, raddr, p, connScope)
	if err != nil {
		connScope.Done()
		return nil, err
	}
	return c, nil
}

func (t *WebsocketTransport) Proxy() bool {
	return false
}

func init() {
	manet.RegisterFromNetAddr(ParseWebsocketNetAddr, "websocket")
	manet.RegisterToNetAddr(ConvertWebsocketMultiaddrToNetAddr, "ws")
	manet.RegisterToNetAddr(ConvertWebsocketMultiaddrToNetAddr, "wss")
}
