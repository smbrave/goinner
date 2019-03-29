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

type Config struct {
	UpAddr      string
	Concurrency int64
}

type Client struct {
	addr   string
	conn   net.Conn
	proxy  *Proxy
	delete func()
}

type Proxy struct {
	config *Config
	exit   chan struct{}

	clients     map[string]*Client
	clientsLock sync.RWMutex
	limit       chan int
}

func NewProxy(config *Config) *Proxy {
	return &Proxy{
		config:  config,
		exit:    make(chan struct{}),
		clients: make(map[string]*Client),
		limit:   make(chan int, config.Concurrency),
	}
}

func (c *Client) Keepalive() {
	connectAddr := ""

	//心跳
	for {
		buf := make([]byte, 4)
		n, err := c.conn.Read(buf)
		if err != nil {
			log.Println("[common] c.conn.Read(buf) error:", err.Error())
			c.delete()
			return
		}
		if n != 4 {
			log.Println("read len!=4")
			c.delete()
			return
		}

		packetLen := binary.BigEndian.Uint32(buf)
		packetBuf := make([]byte, packetLen)
		n, err = c.conn.Read(packetBuf)
		if err != nil {
			log.Println("c.conn.Read(packetBuf) error:", err.Error())
			c.delete()
			return
		}
		if n != int(packetLen) {
			log.Println("c.conn.Read(packetBuf) len:", n, "expect:", packetLen)
			c.delete()
			return
		}

		var packet common.Keepalive
		err = json.Unmarshal(packetBuf, &packet)
		if err != nil {
			log.Println("json.Unmarshal(packetBuf, &packet) error:", err.Error())
			c.delete()
			return
		}

		if packet.Code == common.PACKET_CODE_CONNECT {
			connectAddr = packet.Addr
			break
		}
		//log.Println("[common] PACKET_CODE_KEEPALIVE ok")
	}

	//发起链接
	upDial := net.Dialer{Timeout: 5 * time.Second}
	downConn, err := upDial.Dial("tcp", connectAddr)
	if err != nil {
		log.Printf("[ERROR] connect %s err:%s", connectAddr, err.Error())
		c.delete()
		return
	}

	log.Println("[common] PACKET_CODE_CONNECT", connectAddr)
	downConn.(*net.TCPConn).SetNoDelay(true)
	downConn.(*net.TCPConn).SetKeepAlive(true)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(c.conn, downConn)
		wg.Done()
	}()

	go func() {
		io.Copy(downConn, c.conn)
		wg.Done()
	}()

	wg.Wait()
	downConn.Close()
	c.delete()
}

func (p *Proxy) Acquire() {
	p.limit <- int(1)
}
func (p *Proxy) Release() {
	<-p.limit
}

func (p *Proxy) Connect() {
	p.Acquire()

	upDial := net.Dialer{Timeout: 5 * time.Second}
	upConn, err := upDial.Dial("tcp", p.config.UpAddr)
	if err != nil {
		log.Printf("[ERROR] connect %s err:%s", p.config.UpAddr, err.Error())
		p.Release()
		time.Sleep(time.Second)
		return
	}

	upConn.(*net.TCPConn).SetNoDelay(true)
	upConn.(*net.TCPConn).SetKeepAlive(true)
	localAdrr := upConn.LocalAddr().String()
	client := &Client{
		conn:  upConn,
		addr:  localAdrr,
		proxy: p,
	}
	client.delete = func() {
		p.clientsLock.Lock()
		delete(p.clients, localAdrr)
		p.clientsLock.Unlock()
		p.Release()
		upConn.Close()
	}

	go client.Keepalive()
	p.clientsLock.Lock()
	p.clients[client.addr] = client
	p.clientsLock.Unlock()

}

func (p *Proxy) Start() {
	for {
		p.Connect()
	}
}
