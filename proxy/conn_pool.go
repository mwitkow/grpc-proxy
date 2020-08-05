package proxy

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type Pool struct {
	size int
	ttl  int64

	//  max streams on a *PoolConn
	maxStreams int
	//  max idle conns
	maxIdle int

	sync.Mutex
	conns map[string]*streamsPool
}

type streamsPool struct {
	//  head of list
	head *PoolConn
	//  busy conns list
	busy *PoolConn
	//  the siza of list
	count int
	//  idle conn
	idle int
}

type PoolConn struct {
	//  grpc conn
	*grpc.ClientConn
	err  error
	addr string

	//  pool and streams pool
	pool    *Pool
	sp      *streamsPool
	streams int
	created int64

	//  list
	pre  *PoolConn
	next *PoolConn
	in   bool
}

func NewPool(size int, ttl time.Duration, idle int, ms int) *Pool {
	if ms <= 0 {
		ms = 1
	}
	if idle < 0 {
		idle = 0
	}
	return &Pool{
		size:       size,
		ttl:        int64(ttl.Seconds()),
		maxStreams: ms,
		maxIdle:    idle,
		conns:      make(map[string]*streamsPool),
	}
}

func (p *Pool) GetConn(addr string, opts ...grpc.DialOption) (*PoolConn, error) {
	now := time.Now().Unix()
	p.Lock()
	sp, ok := p.conns[addr]
	if !ok {
		sp = &streamsPool{head: &PoolConn{}, busy: &PoolConn{}, count: 0, idle: 0}
		p.conns[addr] = sp
	}
	//  while we have conns check streams and then return one
	//  otherwise we'll create a new conn
	conn := sp.head.next
	for conn != nil {
		//  check conn state
		// https://github.com/grpc/grpc/blob/master/doc/connectivity-semantics-and-api.md
		switch conn.GetState() {
		case connectivity.Connecting:
			conn = conn.next
			continue
		case connectivity.Shutdown:
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				sp.idle--
			}
			conn = next
			continue
		case connectivity.TransientFailure:
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				conn.ClientConn.Close()
				sp.idle--
			}
			conn = next
			continue
		case connectivity.Ready:
		case connectivity.Idle:
		}
		//  a old conn
		if now-conn.created > p.ttl {
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				conn.ClientConn.Close()
				sp.idle--
			}
			conn = next
			continue
		}
		//  a busy conn
		if conn.streams >= p.maxStreams {
			next := conn.next
			removeConn(conn)
			addConnAfter(conn, sp.busy)
			conn = next
			continue
		}
		//  a idle conn
		if conn.streams == 0 {
			sp.idle--
		}
		//  a good conn
		conn.streams++
		p.Unlock()
		return conn, nil
	}
	p.Unlock()

	// create new conn
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}

	conn = &PoolConn{cc, nil, addr, p, sp, 1, time.Now().Unix(), nil, nil, false}

	//  add conn to streams pool
	p.Lock()
	if sp.count < p.size {
		addConnAfter(conn, sp.head)
	}
	p.Unlock()

	return conn, nil
}

func (p *Pool) Release(addr string, conn *PoolConn, err error) {
	p.Lock()
	p, sp, created := conn.pool, conn.sp, conn.created
	//  try to add conn
	if !conn.in && sp.count < p.size {
		addConnAfter(conn, sp.head)
	}
	if !conn.in {
		p.Unlock()
		conn.ClientConn.Close()
		return
	}
	//  a busy conn
	if conn.streams >= p.maxStreams {
		removeConn(conn)
		addConnAfter(conn, sp.head)
	}
	conn.streams--
	//  if streams == 0, we can do something
	if conn.streams == 0 {
		//  1. it has errored
		//  2. too many idle conn or
		//  3. conn is too old
		now := time.Now().Unix()
		if err != nil || sp.idle >= p.maxIdle || now-created > p.ttl {
			removeConn(conn)
			p.Unlock()
			conn.ClientConn.Close()
			return
		}
		sp.idle++
	}
	p.Unlock()
	return
}

func (conn *PoolConn) Close() {
	conn.pool.Release(conn.addr, conn, conn.err)
}

func removeConn(conn *PoolConn) {
	if conn.pre != nil {
		conn.pre.next = conn.next
	}
	if conn.next != nil {
		conn.next.pre = conn.pre
	}
	conn.pre = nil
	conn.next = nil
	conn.in = false
	conn.sp.count--
	return
}

func addConnAfter(conn *PoolConn, after *PoolConn) {
	conn.next = after.next
	conn.pre = after
	if after.next != nil {
		after.next.pre = conn
	}
	after.next = conn
	conn.in = true
	conn.sp.count++
	return
}
