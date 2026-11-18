package rcon

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func readPkt(c net.Conn) (id, typ int32, body string, ok bool) {
	var h [4]byte
	if _, err := io.ReadFull(c, h[:]); err != nil {
		return
	}
	p := make([]byte, binary.LittleEndian.Uint32(h[:]))
	if _, err := io.ReadFull(c, p); err != nil {
		return
	}
	return int32(binary.LittleEndian.Uint32(p)), int32(binary.LittleEndian.Uint32(p[4:])), string(p[8 : len(p)-2]), true
}

func writePkt(c net.Conn, id, typ int32, body string) {
	b := make([]byte, 14+len(body))
	binary.LittleEndian.PutUint32(b, uint32(10+len(body)))
	binary.LittleEndian.PutUint32(b[4:], uint32(id))
	binary.LittleEndian.PutUint32(b[8:], uint32(typ))
	copy(b[12:], body)
	c.Write(b)
}

func fake(t *testing.T, pass string) string {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		id, _, body, _ := readPkt(c)
		writePkt(c, id, 0, "") // empty value first, like Source servers
		if body != pass {
			writePkt(c, -1, 2, "")
			return
		}
		writePkt(c, id, 2, "")
		for {
			id, _, cmd, ok := readPkt(c)
			if !ok {
				return
			}
			writePkt(c, id, 0, "echo:"+cmd)
		}
	}()
	return l.Addr().String()
}

func TestExec(t *testing.T) {
	cl, err := Dial(fake(t, "pw"), "pw", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	out, err := cl.Exec("players")
	if err != nil || out != "echo:players" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestWrongPassword(t *testing.T) {
	if _, err := Dial(fake(t, "pw"), "bad", time.Second); err == nil {
		t.Fatal("expected auth error")
	}
}
