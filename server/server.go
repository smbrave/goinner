package main

import (
	"fmt"
	"io"
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
}

type Client struct {
	addr string
	conn net.Conn
}

type Server struct {
	config          *Config
	exit            chan struct{}
	idleClients     map[string]*Client
	idleClientsLock sync.RWMutex
}

func NewServer(config *Config) *Server {
	return &Server{
		config:      config,
		exit:        make(chan struct{}),
		idleClients: make(map[string]*Client),
	}
}

func (s *Server) Start() {
	go s.BackendLoop()

	s.FrontLoop()
}

func (s *Server) Copy(src, dst net.Conn) {

	var wg sync.WaitGroup
	wg.Add(1)
	var once sync.Once

	go func() {
		io.Copy(src, dst)
		once.Do(func() {
			wg.Done()
		})
	}()

	go func() {
		io.Copy(dst, src)
		once.Do(func() {
			wg.Done()
		})
	}()

	log.Println("[map] :", src.RemoteAddr().String(), "||", src.LocalAddr().String(), "==>",
		dst.LocalAddr().String(), "||", dst.RemoteAddr().String())

	wg.Wait()

	log.Println("[unmap] :", src.RemoteAddr().String(), "||", src.LocalAddr().String(), "==>",
		dst.LocalAddr().String(), "||", dst.RemoteAddr().String())

	dst.Close()
	src.Close()
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

		if len(s.idleClients) == 0 {
			log.Println("not enough backend client connection")
			conn.Close()
			continue
		}
		log.Println("front connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())

		s.idleClientsLock.Lock()
		clients := make([]*Client, 0)
		for _, v := range s.idleClients {
			clients = append(clients, v)
		}
		cli := clients[rand.Intn(len(clients))]
		delete(s.idleClients, cli.addr)
		s.idleClientsLock.Unlock()

		go s.Copy(conn, cli.conn)

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

		s.idleClientsLock.Lock()
		if client, ok := s.idleClients[remoteAddr]; ok {
			client.conn.Close()
		}
		s.idleClients[remoteAddr] = &Client{conn: conn, addr: conn.RemoteAddr().String()}
		s.idleClientsLock.Unlock()

		log.Println("backend connected:", conn.RemoteAddr().String(), "==>", conn.LocalAddr().String())
	}

	close(s.exit)
}
