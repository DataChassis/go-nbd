package server

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/datachassis/go-nbd/pkg/backend"
)

type ServerOptions struct {
	Options
	ListenOn                 string
	AllowMultipleConnections bool
}

type BackendSupplier func() (backend.Backend, error)

var serverInstance Server = nil

type Server interface {
	Start() error
	Stop() error
	AddExport(name string, description string, b BackendSupplier) error
	RemoveExport(name string) error
	Options() ServerOptions
}

type nbdServer struct {
	opts    ServerOptions
	exports map[string]*Export
	lock    sync.RWMutex
	socket  net.Listener
}

func NewServer(opts ServerOptions) (Server, error) {
	if serverInstance != nil {
		panic("cannot have two server instances")
	}
	s := nbdServer{
		opts:    opts,
		exports: make(map[string]*Export, 10),
		lock:    sync.RWMutex{},
	}
	serverInstance = &s
	return &s, nil
}

func (s *nbdServer) Options() ServerOptions { return s.opts }

func (s *nbdServer) Start() error {
	var err error
	s.socket, err = net.Listen("tcp", s.opts.ListenOn)
	if err != nil {
		panic(err)
	}
	log.Printf("Listening on [%s]", s.socket.Addr())
	go s.startLoop()

	return nil
}

func (s *nbdServer) Stop() error {
	return nil
}

func (s *nbdServer) AddExport(name string, description string, b BackendSupplier) error {
	if strings.ContainsAny(name, " \t") {
		return fmt.Errorf("export name [%s] cannot contain whitespace ", name)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.exports[name]; ok {
		return fmt.Errorf("export [%s] already exists", name)
	}
	log.Printf("requesting backend for export [%s]", name)
	be, err := b()
	if err != nil {
		return fmt.Errorf("error in call to backend supplier for export [%s]: %w", name, err)
	}
	s.exports[name] = &Export{
		Name:        name,
		Description: description,
		Backend:     be,
	}
	log.Printf("added export [%s] with backend %s", name, be.String())

	return nil
}

func (s *nbdServer) RemoveExport(name string) error {
	// TBD: need to check connections and flush file
	return nil
}

func (s *nbdServer) startLoop() {
	for {
		conn, err := s.socket.Accept()
		if err != nil {
			log.Printf("could not accept connection:", err)
			continue
		}
		go s.startServing(conn)
	}
}

func (s *nbdServer) startServing(conn net.Conn) {
	defer s.trapPanic(conn)
	if err := Handle(conn, s.exportsAsSlice(), &s.opts.Options); err != nil {
		panic(err)
	}
}

func (s *nbdServer) trapPanic(conn net.Conn) {
	_ = conn.Close()
	if err := recover(); err != nil {
		log.Printf("client disconnected with error: %v", err)
	}
}

func (s *nbdServer) exportsAsSlice() []Export {
	s.lock.Lock()
	defer s.lock.Unlock()

	exports := make([]Export, 0, len(s.exports))
	for _, v := range s.exports {
		exports = append(exports, *v)
	}
	return exports
}
