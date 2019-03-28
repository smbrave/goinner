package main

import (
	"encoding/binary"
	"encoding/json"
	"github.com/smbrave/goinner/common"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Client struct {
	addr    string
	conn    net.Conn
	delete  func()
	server  *Server
	connect chan net.Conn
}

func (c *Client) SendKeepalive() error {
	var packet common.Keepalive
	packet.Code = common.PACKET_CODE_KEEPALIVE
	buf, _ := json.Marshal(packet)

	bufLen := uint32(len(buf))
	err := binary.Write(c.conn, binary.BigEndian, &bufLen)
	if err != nil {
		log.Println("[common] binary.Write(c.conn, binary.BigEndian, &bufLen) error:", err.Error())
		return err
	}

	n, err := c.conn.Write(buf)
	if err != nil {
		log.Println("c.conn.Write(buf) error:", err.Error())
		return err
	}
	if n != int(bufLen) {
		log.Println("write n:", n, "expect:", bufLen)
		return err
	}
	return nil
}

func (c *Client) SendConn() error {
	var packet common.Keepalive
	packet.Code = common.PACKET_CODE_CONNECT
	packet.Addr = c.server.config.RemoteAddr
	buf, _ := json.Marshal(packet)

	bufLen := uint32(len(buf))
	err := binary.Write(c.conn, binary.BigEndian, &bufLen)
	if err != nil {
		log.Println("binary.Write(c.conn, binary.BigEndian, &bufLen) error:", err.Error())
		return err
	}

	n, err := c.conn.Write(buf)
	if err != nil {
		log.Println("c.conn.Write(buf) error:", err.Error())
		return err
	}
	if n != int(bufLen) {
		log.Println("write n:", n, "expect:", bufLen)
		return err
	}
	return nil
}

func (c *Client) Copy(conn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(1)
	var once sync.Once

	go func() {
		io.Copy(c.conn, conn)
		once.Do(func() {
			wg.Done()
		})
	}()

	go func() {
		io.Copy(conn, c.conn)
		once.Do(func() {
			wg.Done()
		})
	}()

	log.Println("[map] :", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String(), "==>",
		c.conn.LocalAddr().String(), "==>", c.conn.RemoteAddr().String())

	wg.Wait()

	log.Println("[unmap] :", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String(), "==>",
		c.conn.LocalAddr().String(), "==>", c.conn.RemoteAddr().String())

}

func (c *Client) IoLoop() {
	timer := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-timer.C:
			err := c.SendKeepalive()
			if err != nil {
				goto exit
			}
		case conn := <-c.connect:
			err := c.SendConn()
			if err != nil {
				conn.Close()
				goto exit
			}
			c.Copy(conn)
			conn.Close()
			goto exit
		}
	}
exit:
	c.delete()
}
