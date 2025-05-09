package wasmws

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type parsedWebsocketMultiaddr struct {
	isWSS bool
	// sni is the SNI value for the TLS handshake, and for setting HTTP Host header
	sni *ma.Component
	// the rest of the multiaddr before the /tls/sni/example.com/ws or /ws or /wss
	restMultiaddr ma.Multiaddr
}

func parseWebsocketMultiaddr(a ma.Multiaddr) (parsedWebsocketMultiaddr, error) {
	out := parsedWebsocketMultiaddr{}
	// First check if we have a WSS component. If so we'll canonicalize it into a /tls/ws
	withoutWss := a.Decapsulate(wssComponent.Multiaddr())
	if !withoutWss.Equal(a) {
		a = withoutWss.Encapsulate(tlsWsAddr)
	}

	// Remove the ws component
	withoutWs := a.Decapsulate(wsComponent.Multiaddr())
	if withoutWs.Equal(a) {
		return out, fmt.Errorf("not a websocket multiaddr")
	}

	rest := withoutWs
	// If this is not a wss then withoutWs is the rest of the multiaddr
	out.restMultiaddr = withoutWs
	for {
		var head *ma.Component
		rest, head = ma.SplitLast(rest)
		if head == nil || len(rest) == 0 {
			break
		}

		if head.Protocol().Code == ma.P_SNI {
			out.sni = head
		} else if head.Protocol().Code == ma.P_TLS {
			out.isWSS = true
			out.restMultiaddr = rest
			break
		}
	}

	return out, nil
}

// In the parseMultiaddr function
func parseMultiaddr(maddr ma.Multiaddr) (*url.URL, error) {
	parsed, err := parseWebsocketMultiaddr(maddr)
	if err != nil {
		return nil, err
	}

	// Convert restMultiaddr to TCP address
	tcpAddr, err := manet.ToNetAddr(parsed.restMultiaddr)
	if err != nil {
		return nil, fmt.Errorf("invalid TCP multiaddr: %w", err)
	}

	host := tcpAddr.(*net.TCPAddr).IP.String()
	port := tcpAddr.(*net.TCPAddr).Port

	// Use SNI host if present
	if parsed.sni != nil {
		host = parsed.sni.Value()
	}

	scheme := "ws"
	if parsed.isWSS {
		scheme = "wss"
	}

	return &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, port),
	}, nil
}

type Addr struct {
	*url.URL
}

var _ net.Addr = (*Addr)(nil)

// Network returns the network type for a WebSocket, "websocket".
func (addr *Addr) Network() string {
	return "websocket"
}

func ConvertWebsocketMultiaddrToNetAddr(maddr ma.Multiaddr) (net.Addr, error) {
	addr, err := parseMultiaddr(maddr)
	if err != nil {
		// If this is a dummy local addr, return a placeholder
		return &Addr{URL: &url.URL{Scheme: "ws", Host: "0.0.0.0:0"}}, nil
	}
	return &Addr{URL: addr}, nil
}

func ParseWebsocketNetAddr(a net.Addr) (ma.Multiaddr, error) {
	wsa, ok := a.(*Addr)
	if !ok {
		return nil, fmt.Errorf("not a websocket address")
	}

	var (
		tcpma ma.Multiaddr
		err   error
		port  int
		host  = wsa.Hostname()
	)

	// Get the port
	if portStr := wsa.Port(); portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse port '%q': %s", portStr, err)
		}
	} else {
		return nil, fmt.Errorf("invalid port in url: '%q'", wsa.URL)
	}

	// NOTE: Ignoring IPv6 zones...
	// Detect if host is IP address or DNS
	if ip := net.ParseIP(host); ip != nil {
		// Assume IP address
		tcpma, err = manet.FromNetAddr(&net.TCPAddr{
			IP:   ip,
			Port: port,
		})
		if err != nil {
			return nil, err
		}
	} else {
		// Assume DNS name
		tcpma, err = ma.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/%d", host, port))
		if err != nil {
			return nil, err
		}
	}

	wsma, err := ma.NewMultiaddr("/" + wsa.Scheme)
	if err != nil {
		return nil, err
	}

	return tcpma.Encapsulate(wsma), nil
}
