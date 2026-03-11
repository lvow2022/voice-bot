package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	URL string

	Headers http.Header

	TLSConfig *tls.Config

	DialTimeout time.Duration

	HandshakeTimeout time.Duration
}

type Client interface {
	Recv() ([]byte, error)
	SendText(data []byte) error
	SendBinary(data []byte) error
	SendTextJSON(v any) error
	Close() error
}

type wsMessage struct {
	messageType int
	data        []byte
}

type client struct {
	conn *websocket.Conn

	recvCh chan []byte
	sendCh chan wsMessage

	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once
	errOnce   sync.Once
	err       error
}

func NewClient(parent context.Context, config Config) (Client, error) {
	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = 10 * time.Second
	}

	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	dialer := websocket.Dialer{
		HandshakeTimeout: config.HandshakeTimeout,
	}

	if config.TLSConfig != nil {
		dialer.TLSClientConfig = config.TLSConfig
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, config.DialTimeout)
	defer dialCancel()

	conn, _, err := dialer.DialContext(dialCtx, config.URL, config.Headers)
	if err != nil {
		cancel()
		return nil, err
	}

	c := &client{
		conn:   conn,
		recvCh: make(chan []byte, 128),
		sendCh: make(chan wsMessage, 128),
		ctx:    ctx,
		cancel: cancel,
	}

	go c.readLoop()
	go c.writeLoop()

	return c, nil
}

func (c *client) readLoop() {
	defer c.Close()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			c.setErr(err)
			return
		}

		select {
		case <-c.ctx.Done():
			c.setErr(c.ctx.Err())
			return
		case c.recvCh <- msg:
		}
	}
}

func (c *client) writeLoop() {
	defer c.Close()

	for {
		select {
		case <-c.ctx.Done():
			c.setErr(c.ctx.Err())
			return
		case msg := <-c.sendCh:
			if err := c.conn.WriteMessage(msg.messageType, msg.data); err != nil {
				c.setErr(err)
				return
			}
		}
	}
}

func (c *client) Recv() ([]byte, error) {
	select {
	case msg := <-c.recvCh:
		return msg, nil
	case <-c.ctx.Done():
		if err := c.getErr(); err != nil {
			return nil, err
		}
		return nil, c.ctx.Err()
	}
}

func (c *client) SendText(data []byte) error {
	return c.send(websocket.TextMessage, data)
}

func (c *client) SendBinary(data []byte) error {
	return c.send(websocket.BinaryMessage, data)
}

func (c *client) SendTextJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return c.send(websocket.TextMessage, data)
}

func (c *client) send(messageType int, data []byte) error {
	select {
	case c.sendCh <- wsMessage{messageType: messageType, data: data}:
		return nil

	case <-c.ctx.Done():
		if err := c.getErr(); err != nil {
			return err
		}
		return c.ctx.Err()
	}
}

func (c *client) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.Close()
	})
	return nil
}

func (c *client) setErr(err error) {
	if err == nil {
		return
	}
	c.errOnce.Do(func() {
		c.err = err
	})
}

func (c *client) getErr() error {
	return c.err
}
