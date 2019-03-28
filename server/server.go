package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"runtime"
	"strings"
	"sync"
)

type Config struct {
	FrontAddr   string
	BackendAddr string
	RemoteAddr  string
}

type Server struct {
	config      *Config
	exit        chan struct{}
	clients     map[string]*Client
	clientsLock sync.RWMutex
}

func NewServer(config *Config) *Server {
	return &Server{
		config:  config,
		exit:    make(chan struct{}),
		clients: make(map[string]*Client),
	}
}


func (s *Server) Start() {
	go s.BackendLoop()
	s.FrontLoop()
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

		if len(s.clients) == 0 {
			log.Println("not enough backend client connection")
			conn.Close()
			continue
		}
		log.Println("front connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())

		s.clientsLock.Lock()
		clients := make([]*Client, 0)
		for _, v := range s.clients {
			clients = append(clients, v)
		}
		cli := clients[rand.Intn(len(clients))]
		delete(s.clients, cli.addr)
		s.clientsLock.Unlock()
		cli.connect <- conn

	}
	close(s.exit)
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
		remoteAddr := conn.RemoteAddr().String()

		s.clientsLock.Lock()
		if client, ok := s.clients[remoteAddr]; ok {
			client.conn.Close()
		}
		client := &Client{
			conn:    conn,
			server:  s,
			addr:    conn.RemoteAddr().String(),
			connect: make(chan net.Conn),
		}
		s.clients[remoteAddr] = client
		s.clientsLock.Unlock()

		client.delete = func() {
			s.clientsLock.Lock()
			delete(s.clients, remoteAddr)
			s.clientsLock.Unlock()
			client.conn.Close()
		}
		go client.IoLoop()
		log.Println("backend connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())
	}

	close(s.exit)
}
