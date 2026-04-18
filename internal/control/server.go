package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
)

type Handler func(context.Context, Request) Response

type Server struct {
	socketPath string
	handler    Handler
	listener   net.Listener
}

func NewServer(socketPath string, handler Handler) (*Server, error) {
	return &Server{
		socketPath: socketPath,
		handler:    handler,
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	_ = os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = ln

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go s.handleConn(ctx, conn)
		}
	}()

	return nil
}

func (s *Server) Close() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
	return nil
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: err.Error()})
		return
	}

	resp := s.handler(ctx, req)
	_ = json.NewEncoder(conn).Encode(resp)
}
