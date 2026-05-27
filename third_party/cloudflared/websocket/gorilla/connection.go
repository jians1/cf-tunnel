package gorilla

import (
	"bytes"
	"fmt"
	"time"

	gws "github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Conn is a wrapper around gorilla/websocket that implements io.ReadWriter.
// It is kept in a separate package so cmd/app does not pull gorilla/websocket.
type Conn struct {
	*gws.Conn
	log     *zerolog.Logger
	readBuf bytes.Buffer
}

func (c *Conn) Read(p []byte) (int, error) {
	if c.readBuf.Len() > 0 {
		return c.readBuf.Read(p)
	}

	_, message, err := c.Conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	copied := copy(p, message)
	c.readBuf.Write(message[copied:])
	return copied, nil
}

func (c *Conn) Write(p []byte) (int, error) {
	if err := c.Conn.WriteMessage(gws.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.Conn.SetReadDeadline(t); err != nil {
		return fmt.Errorf("error setting read deadline: %w", err)
	}
	if err := c.Conn.SetWriteDeadline(t); err != nil {
		return fmt.Errorf("error setting write deadline: %w", err)
	}
	return nil
}
