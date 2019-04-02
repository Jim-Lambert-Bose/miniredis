package sentinel

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/alicebob/miniredis"
	"github.com/alicebob/miniredis/server"
)

func errWrongNumber(cmd string) string {
	return fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(cmd))
}

// Sentinel - a redis sentinel server implementation.
type Sentinel struct {
	sync.Mutex
	srv      *server.Server
	port     int
	password string
	signal   *sync.Cond
	master   *miniredis.Miniredis
	replicas []*miniredis.Miniredis
}

// connCtx has all state for a single connection.
type connCtx struct {
	authenticated bool // auth enabled and a valid AUTH seen
}

// NewSentinel makes a new, non-started, Miniredis object.
func NewSentinel(opts ...Option) *Sentinel {
	s := Sentinel{}
	s.signal = sync.NewCond(&s)
	o := GetOpts(opts...)
	if o.master != nil {
		s.master = o.master
		s.replicas = []*miniredis.Miniredis{o.master} // set a reasonable default
	}
	if o.replicas != nil {
		s.replicas = o.replicas
	}
	return &s
}

// WithMaster - set the master
func (s *Sentinel) WithMaster(m *miniredis.Miniredis) {
	s.master = m
}

// Master - get the master
func (s *Sentinel) Master() *miniredis.Miniredis {
	return s.master
}

// AddReplica - add a new replica to the existing ones
func (s *Sentinel) AddReplica(r *miniredis.Miniredis) {
	s.replicas = append(s.replicas, r)
}

// SetReplicas - replace all the existing replicas
func (s *Sentinel) SetReplicas(replicas []*miniredis.Miniredis) {
	s.replicas = replicas
}

// Replicas - get the current replicas
func (s *Sentinel) Replicas() []*miniredis.Miniredis {
	return s.replicas
}

// Run creates and Start()s a Sentinel.
func Run() (*Sentinel, error) {
	m := NewSentinel()
	return m, m.Start()
}

// Start starts a server. It listens on a random port on localhost. See also
// Addr().
func (s *Sentinel) Start() error {
	srv, err := server.NewServer(fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return err
	}
	return s.start(srv)
}

// StartAddr runs miniredis with a given addr. Examples: "127.0.0.1:6379",
// ":6379", or "127.0.0.1:0"
func (s *Sentinel) StartAddr(addr string) error {
	srv, err := server.NewServer(addr)
	if err != nil {
		return err
	}
	return s.start(srv)
}

func (s *Sentinel) start(srv *server.Server) error {
	s.Lock()
	defer s.Unlock()
	s.srv = srv
	s.port = srv.Addr().Port

	commandsPing(s)
	return nil
}

// Restart restarts a Close()d server on the same port. Values will be
// preserved.
func (s *Sentinel) Restart() error {
	return s.Start()
}

// Close shuts down a Sentinel.
func (s *Sentinel) Close() {
	s.Lock()

	if s.srv == nil {
		s.Unlock()
		return
	}
	srv := s.srv
	s.srv = nil
	s.Unlock()

	// the OnDisconnect callbacks can lock m, so run Close() outside the lock.
	srv.Close()

}

// RequireAuth makes every connection need to AUTH first. Disable again by
// setting an empty string.
func (s *Sentinel) RequireAuth(pw string) {
	s.Lock()
	defer s.Unlock()
	s.password = pw
}

// Addr returns '127.0.0.1:12345'. Can be given to a Dial(). See also Host()
// and Port(), which return the same things.
func (s *Sentinel) Addr() string {
	s.Lock()
	defer s.Unlock()
	return s.srv.Addr().String()
}

// Host returns the host part of Addr().
func (s *Sentinel) Host() string {
	s.Lock()
	defer s.Unlock()
	return s.srv.Addr().IP.String()
}

// Port returns the (random) port part of Addr().
func (s *Sentinel) Port() string {
	s.Lock()
	defer s.Unlock()
	return strconv.Itoa(s.srv.Addr().Port)
}

// CurrentConnectionCount returns the number of currently connected clients.
func (s *Sentinel) CurrentConnectionCount() int {
	s.Lock()
	defer s.Unlock()
	return s.srv.ClientsLen()
}

// TotalConnectionCount returns the number of client connections since server start.
func (s *Sentinel) TotalConnectionCount() int {
	s.Lock()
	defer s.Unlock()
	return int(s.srv.TotalConnections())
}

// handleAuth returns false if connection has no access. It sends the reply.
func (s *Sentinel) handleAuth(c *server.Peer) bool {
	s.Lock()
	defer s.Unlock()
	if s.password == "" {
		return true
	}
	if !getCtx(c).authenticated {
		c.WriteError("NOAUTH Authentication required.")
		return false
	}
	return true
}

func getCtx(c *server.Peer) *connCtx {
	if c.Ctx == nil {
		c.Ctx = &connCtx{}
	}
	return c.Ctx.(*connCtx)
}

func setAuthenticated(c *server.Peer) {
	getCtx(c).authenticated = true
}
