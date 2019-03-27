package main

import (
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Config struct {
	UpAddr      string
	DownAddr    string
	Concurrency int
}

type Client struct {
	addr string
	conn net.Conn
}

type Proxy struct {
	config *Config
	exit   chan struct{}
	limit  chan int
}

func NewProxy(config *Config) *Proxy {
	return &Proxy{
		config: config,
		exit:   make(chan struct{}),
		limit:  make(chan int, config.Concurrency),
	}
}

func (p *Proxy) connect() {
	p.limit <- int(1)

	var once sync.Once
	unLimit := func() {
		<-p.limit
	}

	upDial := net.Dialer{Timeout: 5 * time.Second}
	upConn, err := upDial.Dial("tcp", p.config.UpAddr)
	if err != nil {
		log.Printf("[ERROR] connect %s err:%s", p.config.UpAddr, err.Error())
		time.Sleep(time.Duration(rand.Intn(10)) * time.Second)
		once.Do(unLimit)
		return
	}

	upConn.(*net.TCPConn).SetNoDelay(true)
	upConn.(*net.TCPConn).SetKeepAlive(true)

	downDial := net.Dialer{Timeout: 5 * time.Second}
	downConn, err := downDial.Dial("tcp", p.config.DownAddr)
	if err != nil {
		time.Sleep(time.Duration(rand.Intn(10)) * time.Second)
		log.Printf("[ERROR] connect %s err:%s", p.config.DownAddr, err.Error())
		once.Do(unLimit)
		return
	}

	downConn.(*net.TCPConn).SetNoDelay(true)
	downConn.(*net.TCPConn).SetKeepAlive(true)

	log.Println("connected:", downConn.LocalAddr().String(), "==>", downConn.RemoteAddr().String())
	log.Println("connected:", upConn.LocalAddr().String(), "==>", upConn.RemoteAddr().String())
	go func() {
		io.Copy(upConn, downConn)
		once.Do(unLimit)
	}()

	go func() {
		io.Copy(downConn, upConn)
		once.Do(unLimit)
	}()

}

func (p *Proxy) Start() {
	for {
		p.connect()
	}
}
