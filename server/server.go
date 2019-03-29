package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Config struct {
	FrontAddr   string
	BackendAddr string
	RemoteAddr  string
}

type Server struct {
	config          *Config
	exitChan        chan struct{}
	idleClients     map[string]*Client
	busyClients     map[string]*Client
	idleClientsLock sync.RWMutex
	busyClientsLock sync.RWMutex

	backendListener  net.Listener
	frontendListener net.Listener
}

func NewServer(config *Config) *Server {
	return &Server{
		config:      config,
		exitChan:    make(chan struct{}),
		idleClients: make(map[string]*Client),
		busyClients: make(map[string]*Client),
	}
}

func (s *Server) Stop() {
	close(s.exitChan)
}

func (s *Server) Start() {
	go s.BackendLoop()
	go s.FrontLoop()

	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ticker.C:
			s.PrintInfo()
		case <-s.exitChan:
			goto stop
		}
	}

stop:
	s.busyClientsLock.Lock()
	for _, c := range s.busyClients {
		c.Stop()
	}
	s.busyClientsLock.Unlock()

	s.idleClientsLock.Lock()
	for _, c := range s.idleClients {
		c.Stop()
	}
	s.idleClientsLock.Unlock()

	s.frontendListener.Close()
	s.backendListener.Close()
}

func (s *Server) PrintInfo() {

	log.Println("=======================[proxy]=======================")
	log.Println("=======================", s.config.FrontAddr, "||", s.config.BackendAddr, "||", s.config.RemoteAddr)
	log.Println("=======================", "busy:", len(s.busyClients), "idle:", len(s.idleClients))

	s.busyClientsLock.Lock()
	for _, c := range s.busyClients {
		log.Printf("[busy] connTime[%s] mapTime[%s] [%s ==> %s ==> %s ==> %s]",
			c.connTime.Format("2006-01-02 15:04:05"),
			c.mapTime.Format("2006-01-02 15:04:05"),
			c.frontendConn.RemoteAddr().String(),
			c.frontendConn.LocalAddr().String(),
			c.backendConn.RemoteAddr().String(),
			c.backendConn.LocalAddr().String(),
		)
	}
	s.busyClientsLock.Unlock()

	s.idleClientsLock.Lock()
	for _, c := range s.idleClients {
		log.Printf("[idle] connTime[%s]  [%s ==> %s]",
			c.connTime.Format("2006-01-02 15:04:05"),
			c.backendConn.RemoteAddr().String(),
			c.backendConn.LocalAddr().String(),
		)
	}
	s.idleClientsLock.Unlock()

}

//添加后端链接过来的客户端
func (s *Server) AddBackendClient(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()

	client := NewClient(s)
	client.mapKey = remoteAddr
	client.backendConn = conn

	//删除可能重复的空闲客户端
	s.idleClientsLock.Lock()
	if cli, ok := s.idleClients[remoteAddr]; ok {
		cli.Stop()
	}
	s.idleClients[remoteAddr] = client
	s.idleClientsLock.Unlock()

	//删除可能重复的繁忙客户端
	s.busyClientsLock.Lock()
	if cli, ok := s.busyClients[remoteAddr]; ok {
		cli.Stop()
	}
	s.busyClientsLock.Unlock()

	//创建退出删除函数
	client.delete = func() {
		s.idleClientsLock.Lock()
		delete(s.idleClients, remoteAddr)
		s.idleClientsLock.Unlock()

		s.busyClientsLock.Lock()
		delete(s.busyClients, remoteAddr)
		s.busyClientsLock.Unlock()
	}

	go client.IoLoop()
}

func (s *Server) GetBackendClient() *Client {
	if len(s.idleClients) == 0 {
		log.Println("not enough backend client connection")
		return nil
	}

	s.idleClientsLock.Lock()
	clients := make([]*Client, 0)
	for _, v := range s.idleClients {
		clients = append(clients, v)
	}
	cli := clients[rand.Intn(len(clients))]
	delete(s.idleClients, cli.mapKey)
	s.idleClientsLock.Unlock()
	return cli
}

func (s *Server) FrontLoop() {
	frontAddr, err := net.ResolveTCPAddr("tcp", s.config.FrontAddr)
	if err != nil {
		panic(fmt.Sprintf("addr:%s err:%s", s.config.FrontAddr, err.Error()))
	}

	listener, err := net.ListenTCP("tcp", frontAddr)
	if err != nil {
		panic(fmt.Sprintf("addr:%s err:%s", s.config.BackendAddr, err.Error()))
	}
	s.frontendListener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				log.Printf("NOTICE: temporary Accept() failure - %s", err.Error())
				runtime.Gosched()
				continue
			}

			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("ERROR: listener.Accept() - %s", err.Error())
			}
			break
		}

		conn.(*net.TCPConn).SetNoDelay(true)
		backend := s.GetBackendClient()
		if backend == nil {
			conn.Close()
			continue
		}

		log.Println("front connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())
		go backend.MapConn(conn)

		s.busyClientsLock.Lock()
		s.busyClients[backend.mapKey] = backend
		s.busyClientsLock.Unlock()

	}

}

func (s *Server) BackendLoop() {
	backendAddr, err := net.ResolveTCPAddr("tcp", s.config.BackendAddr)
	if err != nil {
		panic(fmt.Sprintf("addr:%s err:%s", s.config.BackendAddr, err.Error()))
	}

	listener, err := net.ListenTCP("tcp", backendAddr)
	if err != nil {
		panic(fmt.Sprintf("addr:%s err:%s", s.config.BackendAddr, err.Error()))
	}
	s.backendListener = listener
	for {
		conn, err := listener.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				log.Printf("NOTICE: temporary Accept() failure - %s", err.Error())
				runtime.Gosched()
				continue
			}

			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("ERROR: listener.Accept() - %s", err.Error())
			}
			break
		}

		conn.(*net.TCPConn).SetNoDelay(true)
		s.AddBackendClient(conn)
		log.Println("backend connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())
	}

}
