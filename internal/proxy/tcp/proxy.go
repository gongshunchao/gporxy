package tcp

import (
	"context"
	"io"
	"net"
	"sync"

	"gproxy/internal/config"
	"gproxy/internal/proxy/common"
)

type Proxy struct {
	entry    config.Entry
	listener net.Listener
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func New(entry config.Entry) (*Proxy, error) {
	return &Proxy{entry: entry}, nil
}

func (p *Proxy) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", p.entry.Listen)
	if err != nil {
		return err
	}
	p.listener = ln

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer ln.Close()

		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			p.wg.Add(1)
			go func(client net.Conn) {
				defer p.wg.Done()
				defer client.Close()

				target, err := net.Dial("tcp", p.entry.Target)
				if err != nil {
					return
				}
				defer target.Close()

				var relayWG sync.WaitGroup
				relayWG.Add(2)

				go func() {
					defer relayWG.Done()
					bufPtr := common.RelayBufferPool.Get().(*[]byte)
					defer common.RelayBufferPool.Put(bufPtr)
					_, _ = io.CopyBuffer(target, client, *bufPtr)
					closeWrite(target)
					closeRead(client)
				}()

				go func() {
					defer relayWG.Done()
					bufPtr := common.RelayBufferPool.Get().(*[]byte)
					defer common.RelayBufferPool.Put(bufPtr)
					_, _ = io.CopyBuffer(client, target, *bufPtr)
					closeWrite(client)
					closeRead(target)
				}()

				relayWG.Wait()
			}(conn)
		}
	}()

	return nil
}

func (p *Proxy) ListenAddr() string {
	if p.listener == nil {
		return ""
	}

	return p.listener.Addr().String()
}

func (p *Proxy) Close() error {
	_ = p.StopAccepting()
	p.wg.Wait()
	return nil
}

func (p *Proxy) StopAccepting() error {
	var err error
	p.stopOnce.Do(func() {
		if p.listener != nil {
			err = p.listener.Close()
		}
	})
	return err
}

func closeRead(conn net.Conn) {
	type closeReader interface {
		CloseRead() error
	}

	if tcpConn, ok := conn.(closeReader); ok {
		_ = tcpConn.CloseRead()
	}
}

func closeWrite(conn net.Conn) {
	type closeWriter interface {
		CloseWrite() error
	}

	if tcpConn, ok := conn.(closeWriter); ok {
		_ = tcpConn.CloseWrite()
	}
}
