package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/smbrave/goinner/common"
)

type Client struct {
	connTime     time.Time
	mapTime      time.Time
	backendConn  net.Conn
	frontendConn net.Conn
	mapKey       string
	delete       func()
	server       *Server
	connChan     chan net.Conn
	exitChan     chan struct{}
}

func NewClient(s *Server) *Client {
	return &Client{
		connTime:     time.Now(),
		backendConn:  nil,
		frontendConn: nil,
		delete:       nil,
		server:       s,
		connChan:     make(chan net.Conn),
		exitChan:     make(chan struct{}),
	}
}

func (c *Client) SendKeepalive() error {
	var packet common.Keepalive
	packet.Code = common.PACKET_CODE_KEEPALIVE
	buf, _ := json.Marshal(packet)

	bufLen := uint32(len(buf))
	err := binary.Write(c.backendConn, binary.BigEndian, &bufLen)
	if err != nil {
		log.Println("[common] binary.Write(c.conn, binary.BigEndian, &bufLen) error:", err.Error())
		return err
	}

	n, err := c.backendConn.Write(buf)
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
	err := binary.Write(c.backendConn, binary.BigEndian, &bufLen)
	if err != nil {
		log.Println("binary.Write(c.conn, binary.BigEndian, &bufLen) error:", err.Error())
		return err
	}

	n, err := c.backendConn.Write(buf)
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
		io.Copy(c.backendConn, conn)
		once.Do(func() {
			wg.Done()
		})
	}()

	go func() {
		io.Copy(conn, c.backendConn)
		once.Do(func() {
			wg.Done()
		})
	}()

	log.Println("[map] :", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String(), "==>",
		c.backendConn.LocalAddr().String(), "==>", c.backendConn.RemoteAddr().String())

	wg.Wait()

	log.Println("[unmap] :", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String(), "==>",
		c.backendConn.LocalAddr().String(), "==>", c.backendConn.RemoteAddr().String())

}
func (c *Client) MapConn(conn net.Conn) {
	c.mapTime = time.Now()
	c.frontendConn = conn
	c.connChan <- conn
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

		case <-c.exitChan:
			goto exit

		case conn := <-c.connChan:
			err := c.SendConn()
			if err != nil {
				goto exit
			}
			c.Copy(conn)
			goto exit
		}
	}
exit:
	c.Stop()
}

func (c *Client) Stop() {
	if c.exitChan != nil {
		close(c.exitChan)
		c.exitChan = nil
	}
	if c.frontendConn != nil {

		c.frontendConn.Close()
		c.frontendConn = nil
	}
	if c.backendConn != nil {
		c.backendConn.Close()
		c.backendConn = nil
	}

	if c.delete != nil {
		c.delete()
		c.delete = nil
	}
}
