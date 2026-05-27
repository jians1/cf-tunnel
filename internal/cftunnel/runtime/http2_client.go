package runtime

import (
	"fmt"
	"net/http"

	"golang.org/x/net/http2"
)

type HTTP2Client struct {
	conn *http2.ClientConn
}

func NewHTTP2Client(server *HTTP2Server) (*HTTP2Client, error) {
	if server == nil {
		return nil, fmt.Errorf("nil http2 server")
	}

	transport := http2.Transport{}
	conn, err := transport.NewClientConn(server.EdgeConn())
	if err != nil {
		return nil, fmt.Errorf("new http2 client conn: %w", err)
	}

	return &HTTP2Client{conn: conn}, nil
}

func (c *HTTP2Client) RoundTrip(req *http.Request) (*http.Response, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nil http2 client")
	}
	return c.conn.RoundTrip(req)
}

func (c *HTTP2Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
