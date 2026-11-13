package gvisor

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	clientpkg "github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/ippool"
	"github.com/mythologyli/zju-connect/internal/zcdns"
	"github.com/mythologyli/zju-connect/log"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

type Stack struct {
	lifecycleMu              sync.Mutex
	started, reading, closed bool
	closeOnce                sync.Once
	readDone                 chan struct{}
	failures                 chan error
	gvisorStack              *stack.Stack
	resolve                  zcdns.LocalServer
	ipPool                   *ippool.IPPool[[]clientpkg.DomainResource]

	endpoint *Endpoint
	ipMu     sync.Mutex
	ip       tcpip.Address
}

const NICID tcpip.NICID = 1
const MTU uint32 = 1400
const maxInboundPacketSize = 1500

type Endpoint struct {
	client clientpkg.Client

	l3Conn io.ReadWriteCloser
	connMu sync.RWMutex
	fail   func(error)

	dispatcher stack.NetworkDispatcher
}

func (ep *Endpoint) ParseHeader(*stack.PacketBuffer) bool {
	return true
}

func (ep *Endpoint) MTU() uint32 {
	return MTU
}

func (ep *Endpoint) SetMTU(mtu uint32) {
	log.Printf("don't support change MTU from %d to %d", MTU, mtu)
}

func (ep *Endpoint) MaxHeaderLength() uint16 {
	return 0
}

func (ep *Endpoint) LinkAddress() tcpip.LinkAddress {
	return ""
}

func (ep *Endpoint) SetLinkAddress(addr tcpip.LinkAddress) {}

func (ep *Endpoint) Capabilities() stack.LinkEndpointCapabilities {
	return stack.CapabilityNone
}

func (ep *Endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	ep.dispatcher = dispatcher
}

func (ep *Endpoint) IsAttached() bool {
	return ep.dispatcher != nil
}

func (ep *Endpoint) Wait() {}

func (ep *Endpoint) ARPHardwareType() header.ARPHardwareType {
	return header.ARPHardwareNone
}

func (ep *Endpoint) AddHeader(*stack.PacketBuffer) {}

func (ep *Endpoint) Close() {}

func (ep *Endpoint) SetOnCloseAction(func()) {}

// WritePackets is called when get packets from gVisor stack. Then it sends them to VPN server
func (ep *Endpoint) WritePackets(list stack.PacketBufferList) (int, tcpip.Error) {
	ep.connMu.RLock()
	conn := ep.l3Conn
	ep.connMu.RUnlock()
	if conn == nil {
		return 0, &tcpip.ErrClosedForSend{}
	}
	sent := 0
	for _, packet := range list.AsSlice() {
		buf := joinPacketSlices(packet.AsSlices())
		n, err := conn.Write(buf)
		if errors.Is(err, clientpkg.ErrResourceNotFound) {
			sent++
			continue
		}
		if err == nil && n != len(buf) {
			err = io.ErrShortWrite
		}
		if err != nil {
			if ep.fail != nil {
				ep.fail(err)
			}
			return sent, &tcpip.ErrAborted{}
		}
		sent++
	}
	return sent, nil
}
func joinPacketSlices(slices [][]byte) []byte {
	total := 0
	for _, slice := range slices {
		total += len(slice)
	}
	buf := make([]byte, total)
	offset := 0
	for _, slice := range slices {
		offset += copy(buf[offset:], slice)
	}
	return buf
}

func NewStack(client clientpkg.Client) (*Stack, error) {
	s := &Stack{failures: make(chan error, 1), readDone: make(chan struct{})}

	s.gvisorStack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        true,
	})

	s.endpoint = &Endpoint{
		client: client, fail: s.fail,
	}

	tcpipErr := s.gvisorStack.CreateNIC(NICID, s.endpoint)
	if tcpipErr != nil {
		_ = s.Close()
		return nil, errors.New(tcpipErr.String())
	}

	ip, err := client.IP()
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	addr := tcpip.AddrFromSlice(ip)
	s.ip = addr
	protoAddr := tcpip.ProtocolAddress{
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   addr,
			PrefixLen: 32,
		},
		Protocol: ipv4.ProtocolNumber,
	}

	tcpipErr = s.gvisorStack.AddProtocolAddress(NICID, protoAddr, stack.AddressProperties{})
	if tcpipErr != nil {
		_ = s.Close()
		return nil, errors.New(tcpipErr.String())
	}
	clientpkg.RegisterIPUpdateHandler(client, s.updateIP)

	sOpt := tcpip.TCPSACKEnabled(true)
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &sOpt)
	cOpt := tcpip.CongestionControlOption("cubic")
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &cOpt)
	s.gvisorStack.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: NICID})

	return s, nil
}

func (s *Stack) updateIP(ip net.IP) error {
	ip = ip.To4()
	if ip == nil {
		return errors.New("virtual IP update is not IPv4")
	}
	newAddr := tcpip.AddrFromSlice(ip)
	s.ipMu.Lock()
	defer s.ipMu.Unlock()
	if newAddr == s.ip {
		return nil
	}
	protoAddr := tcpip.ProtocolAddress{
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: newAddr, PrefixLen: 32},
		Protocol:          ipv4.ProtocolNumber,
	}
	if err := s.gvisorStack.AddProtocolAddress(NICID, protoAddr, stack.AddressProperties{}); err != nil {
		return errors.New(err.String())
	}
	if err := s.gvisorStack.RemoveAddress(NICID, s.ip); err != nil {
		_ = s.gvisorStack.RemoveAddress(NICID, newAddr)
		return errors.New(err.String())
	}
	s.ip = newAddr
	return nil
}

func (s *Stack) SetupResolve(r zcdns.LocalServer) {
	s.resolve = r
}

func (s *Stack) SetupIPPool(ipPool *ippool.IPPool[[]clientpkg.DomainResource]) {
	s.ipPool = ipPool
}

// Run is retained for protocol callers; the CLI uses Start/Errors/Close.
func (s *Stack) Run() {
	if err := s.Start(context.Background()); err != nil {
		s.fail(err)
		return
	}
	<-s.readDone
}

func (s *Stack) Errors() <-chan error { return s.failures }

func (s *Stack) fail(err error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.failures <- err:
	default:
	}
}

func (s *Stack) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	if s.closed || s.started {
		s.lifecycleMu.Unlock()
		return net.ErrClosed
	}
	s.started = true
	s.lifecycleMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		if closer, ok := s.endpoint.client.(interface{ Close() }); ok {
			closer.Close()
		}
	})
	defer stop()
	conn, err := s.endpoint.client.NewL3Conn()
	if err != nil {
		return err
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed || ctx.Err() != nil {
		conn.Close()
		return net.ErrClosed
	}
	s.endpoint.connMu.Lock()
	s.endpoint.l3Conn = conn
	s.endpoint.connMu.Unlock()
	s.reading = true
	go s.readPackets(conn)
	return nil
}

func (s *Stack) readPackets(conn io.ReadWriteCloser) {
	defer close(s.readDone)
	buf := make([]byte, maxInboundPacketSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			s.fail(err)
			return
		}
		if n <= 0 {
			s.fail(io.ErrNoProgress)
			return
		}
		packet := makeInboundPacketBuffer(buf, n)
		s.endpoint.dispatcher.DeliverNetworkPacket(header.IPv4ProtocolNumber, packet)
		packet.DecRef()
	}
}

func (s *Stack) Close() error {
	s.closeOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.closed = true
		reading := s.reading
		s.lifecycleMu.Unlock()
		s.endpoint.connMu.RLock()
		conn := s.endpoint.l3Conn
		s.endpoint.connMu.RUnlock()
		if conn != nil {
			_ = conn.Close()
		}
		if reading {
			<-s.readDone
		}
		s.gvisorStack.Close()
		s.gvisorStack.Wait()
	})
	return nil
}
func makeInboundPacketBuffer(buf []byte, n int) *stack.PacketBuffer {
	return stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(buf[:n]),
	})
}
