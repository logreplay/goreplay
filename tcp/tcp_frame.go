package tcp

import (
	"math/big"
	"net"
	"sync"

	"goreplay/logger"
	"goreplay/tcp/ack"

	"github.com/google/gopacket/layers"
)

// FramerBuilder 通常每个连接Build一个Framer
type FramerBuilder interface {
	New(string) Framer
}

// Framer interface of the framer
type Framer interface {
	// Start hints message pool to start the reassembling the message
	Start(*Packet) (isIncoming, isOutgoing bool)

	// End hints message pool to stop the session
	End(*Message) bool

	// ReqRspKey request and response match key
	ReqRspKey(pckt *Packet) string

	// MessageKey generate key for msg
	MessageKey(pckt *Packet, isPeer bool) *big.Int

	// MessageGroupBy group by the packet by message key
	MessageGroupBy(p *Packet) map[string]*Packet
}

var (
	framers sync.Map
)

// RegisterFramerBuilder registers the protocol builder
func RegisterFramerBuilder(protocol string, framerBuilder FramerBuilder) {
	framers.Store(protocol, framerBuilder)
}

// GetFramerBuilder gets a builder for some protocol
func GetFramerBuilder(protocol string) FramerBuilder {
	if val, ok := framers.Load(protocol); ok {
		return val.(FramerBuilder)
	}

	return nil
}

// CommonFramer a common framer
type CommonFramer struct {
	ListenAddr string
}

// ReqRspKey key for both req and rsp
func (c *CommonFramer) ReqRspKey(pckt *Packet) string {
	isOut := pckt.Src() == c.ListenAddr
	// if response get peer key, keep it is the same of request key
	return c.MessageKey(pckt, isOut).String()
}

// MessageKey key of the protocol
func (c *CommonFramer) MessageKey(pckt *Packet, isPeer bool) *big.Int {
	return DefaultMessageKey(pckt, isPeer)
}

// MessageGroupBy group by the packet by message key
func (c *CommonFramer) MessageGroupBy(p *Packet) map[string]*Packet {
	return map[string]*Packet{c.MessageKey(p, false).String(): p}
}

// IsStart is the starting of a tcp packet
func (c *CommonFramer) IsStart(pckt *Packet) bool {
	// need get peer last ack, so the key should be peer's key
	key := c.MessageKey(pckt, true)
	in, out := c.InOut(pckt)
	logger.Debug3("ListenAddr: ", c.ListenAddr, "dst: ", pckt.Dst(), "src: ", pckt.Src())
	return IsStart(pckt, key.String(), in, out)
}

// CheckLength check the length of payload
func (c *CommonFramer) CheckLength(payload []byte, length int) bool {
	return len(payload) >= length
}

// InOut return isIn and isOut by parsing the packet
func (c *CommonFramer) InOut(pckt *Packet) (bool, bool) {
	return IsRequest(pckt, c.ListenAddr), IsResponse(pckt, c.ListenAddr)
}

// DefaultMessageKey generate a tcp connection transfer message identify key
// isPeer is true will get peer's key
func DefaultMessageKey(pckt *Packet, isPeer bool) *big.Int {
	var (
		srcIP   net.IP
		srcPort layers.TCPPort
		dstPort layers.TCPPort
	)

	if isPeer {
		srcIP = pckt.DstIP
		srcPort = pckt.DstPort
		dstPort = pckt.SrcPort
	} else {
		srcIP = pckt.SrcIP
		srcPort = pckt.SrcPort
		dstPort = pckt.DstPort
	}

	lst := len(srcIP) - 4

	// uint64(srcPort)<<48 | uint64(dstPort)<<32 | uint64(_uint32(srcIP[lst:]))
	bigInt := big.NewInt(int64(srcPort) << 48)
	bigInt.Or(bigInt, big.NewInt(int64(dstPort)<<32))
	bigInt.Or(bigInt, big.NewInt(int64(_uint32(srcIP[lst:]))))

	return bigInt
}

// IsStart a packet seq is equals the peer last ack that is the beginning of frame
func IsStart(pckt *Packet, key string, in, out bool) bool {
	var seq uint32

	if in {
		// get last response ack from cache, it it next request seq
		seq = ack.GetServerAck(key)
	} else if out {
		// get last request ack from cache, it is next response seq
		seq = ack.GetClientAck(key)
	}

	logger.Debug3("IsStart key:", key, "seq:", seq, "pckt.Seq:", pckt.Seq)

	// response seq should be equals request ack
	// request seq should be equals last response ack
	return pckt.Seq == seq
}
