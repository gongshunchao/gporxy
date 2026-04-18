package udp

import (
	"context"
	"errors"
	"hash/fnv"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gproxy/internal/config"
)

const shardCount = 16

type session struct {
	clientAddr net.Addr
	targetConn *net.UDPConn
	lastSeen   time.Time
}

type sessionShard struct {
	mu       sync.Mutex
	sessions map[string]*session
}

type Proxy struct {
	entry       config.Entry
	idleTimeout time.Duration
	conn        net.PacketConn
	shards      [shardCount]sessionShard
	accepting   atomic.Bool
	closeOnce   sync.Once
	done        chan struct{}
	wg          sync.WaitGroup
}

func New(entry config.Entry, idleTimeout time.Duration) (*Proxy, error) {
	p := &Proxy{
		entry:       entry,
		idleTimeout: idleTimeout,
		done:        make(chan struct{}),
	}

	for i := range p.shards {
		p.shards[i].sessions = make(map[string]*session)
	}
	p.accepting.Store(true)

	return p, nil
}

func (p *Proxy) Start(ctx context.Context) error {
	conn, err := net.ListenPacket("udp", p.entry.Listen)
	if err != nil {
		return err
	}
	p.conn = conn

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			p.handlePacket(buf[:n], addr)
		}
	}()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.cleanupInterval())
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-p.done:
				return
			case <-ticker.C:
				p.cleanupIdle()
			}
		}
	}()

	return nil
}

func (p *Proxy) ListenAddr() string {
	if p.conn == nil {
		return ""
	}
	return p.conn.LocalAddr().String()
}

func (p *Proxy) SessionCount() int {
	total := 0
	for i := range p.shards {
		shard := &p.shards[i]
		shard.mu.Lock()
		total += len(shard.sessions)
		shard.mu.Unlock()
	}
	return total
}

func (p *Proxy) Close() error {
	p.closeOnce.Do(func() {
		close(p.done)
	})

	if p.conn != nil {
		_ = p.conn.Close()
	}

	for i := range p.shards {
		shard := &p.shards[i]
		shard.mu.Lock()
		for key, sess := range shard.sessions {
			_ = sess.targetConn.Close()
			delete(shard.sessions, key)
		}
		shard.mu.Unlock()
	}

	p.wg.Wait()
	return nil
}

func (p *Proxy) StopAccepting() error {
	p.accepting.Store(false)
	return nil
}

func (p *Proxy) handlePacket(payload []byte, clientAddr net.Addr) {
	sess, err := p.getOrCreateSession(clientAddr)
	if err != nil {
		return
	}

	shard := p.shardFor(clientAddr.String())
	shard.mu.Lock()
	sess.lastSeen = time.Now()
	shard.mu.Unlock()

	_, _ = sess.targetConn.Write(payload)
}

func (p *Proxy) getOrCreateSession(clientAddr net.Addr) (*session, error) {
	key := clientAddr.String()
	shard := p.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if existing, ok := shard.sessions[key]; ok {
		return existing, nil
	}

	if !p.accepting.Load() {
		return nil, errors.New("udp proxy is stopping")
	}

	targetAddr, err := net.ResolveUDPAddr("udp", p.entry.Target)
	if err != nil {
		return nil, err
	}

	targetConn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return nil, err
	}

	sess := &session{
		clientAddr: clientAddr,
		targetConn: targetConn,
		lastSeen:   time.Now(),
	}
	shard.sessions[key] = sess

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		buf := make([]byte, 64*1024)
		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				return
			}
			_, _ = p.conn.WriteTo(buf[:n], clientAddr)

			shard.mu.Lock()
			if current, ok := shard.sessions[key]; ok && current == sess {
				current.lastSeen = time.Now()
			}
			shard.mu.Unlock()
		}
	}()

	return sess, nil
}

func (p *Proxy) cleanupIdle() {
	deadline := time.Now().Add(-p.idleTimeout)

	for i := range p.shards {
		shard := &p.shards[i]
		shard.mu.Lock()
		for key, sess := range shard.sessions {
			if sess.lastSeen.Before(deadline) {
				_ = sess.targetConn.Close()
				delete(shard.sessions, key)
			}
		}
		shard.mu.Unlock()
	}
}

func (p *Proxy) cleanupInterval() time.Duration {
	if p.idleTimeout <= 0 {
		return time.Second
	}
	interval := p.idleTimeout / 2
	if interval <= 0 {
		return time.Millisecond
	}
	return interval
}

func (p *Proxy) shardFor(key string) *sessionShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &p.shards[h.Sum32()%shardCount]
}
