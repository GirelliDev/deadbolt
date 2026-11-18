// Package rcon is a minimal Source RCON client (what Project Zomboid speaks).
package rcon

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	typeAuth     = 3
	typeExec     = 2
	typeAuthResp = 2
	typeResp     = 0
)

type Client struct {
	conn    net.Conn
	timeout time.Duration
	nextID  int32
}

// Dial connects and authenticates.
func Dial(addr, password string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, timeout: timeout, nextID: 1}
	id := c.id()
	if err := c.write(id, typeAuth, password); err != nil {
		conn.Close()
		return nil, err
	}
	for { // some servers send an empty RESPONSE_VALUE before the auth reply
		rid, typ, _, err := c.read()
		if err != nil {
			conn.Close()
			return nil, err
		}
		if typ == typeAuthResp {
			if rid == -1 {
				conn.Close()
				return nil, errors.New("rcon: wrong password")
			}
			return c, nil
		}
	}
}

func (c *Client) Close() error { return c.conn.Close() }

// Exec runs one command and returns its output.
func (c *Client) Exec(cmd string) (string, error) {
	id := c.id()
	if err := c.write(id, typeExec, cmd); err != nil {
		return "", err
	}
	for {
		rid, typ, body, err := c.read()
		if err != nil {
			return "", err
		}
		if typ == typeResp && rid == id {
			return body, nil
		}
	}
}

func (c *Client) id() int32 { c.nextID++; return c.nextID }

func (c *Client) write(id, typ int32, body string) error {
	buf := make([]byte, 14+len(body))
	binary.LittleEndian.PutUint32(buf[0:], uint32(10+len(body)))
	binary.LittleEndian.PutUint32(buf[4:], uint32(id))
	binary.LittleEndian.PutUint32(buf[8:], uint32(typ))
	copy(buf[12:], body)
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	_, err := c.conn.Write(buf)
	return err
}

func (c *Client) read() (id, typ int32, body string, err error) {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	var hdr [4]byte
	if _, err = io.ReadFull(c.conn, hdr[:]); err != nil {
		return
	}
	size := binary.LittleEndian.Uint32(hdr[:])
	if size < 10 || size > 1<<20 {
		return 0, 0, "", fmt.Errorf("rcon: bad packet size %d", size)
	}
	p := make([]byte, size)
	if _, err = io.ReadFull(c.conn, p); err != nil {
		return
	}
	id = int32(binary.LittleEndian.Uint32(p[0:]))
	typ = int32(binary.LittleEndian.Uint32(p[4:]))
	body = string(p[8 : size-2])
	return
}
