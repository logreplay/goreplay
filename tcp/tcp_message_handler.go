package tcp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"goreplay/logger"
	"goreplay/size"
)

// MessageHandler 从 capture 包接收 tcp 报文, 使用不同 messagePool 处理
type MessageHandler struct {
	pool        sync.Map     // key 为"ip:port", value 为 *MessagePool
	poolLen     uint         // pool 里元素的个数
	lastHandler *MessagePool // 保存 pool 里的一个元素。大部分情况下 pool 里只有一个元素，此时直接返回 lastHandler 即可，避免从 pool 中查询。
}

// MessagePoolParam 初始化MessagePool的参数
type MessagePoolParam struct {
	// MaxSize 由 CopyBufferSize  指定
	MaxSize size.Size
	// MessageExpire 内存缓存tcp报文的过期时间
	MessageExpire time.Duration
	// Handler 解析报文后的处理函数
	Handler Handler
	// TrackResponse 是否录制响应
	TrackResponse bool
	// InTarget 进流量的录制目标
	InTarget ListenTarget
	// 出流量的录制目标
	OutTarget []ListenTarget
}

// ListenTarget 录制目标
type ListenTarget struct {
	Address  string // 录制服务地址，IP:PORT
	Protocol string // 对应的协议
}

// NewMessageHandler 初始化实例
func NewMessageHandler(pp MessagePoolParam) *MessageHandler {
	m := &MessageHandler{}
	m.initInTarget(pp)
	m.initOutTarget(pp)

	m.pool.Range(func(key, value interface{}) bool {
		logger.Info("key: ", key)
		return true
	})
	return m
}

// store 注意：非并发安全
func (h *MessageHandler) store(addr string, handler *MessagePool) {
	h.pool.Store(addr, handler)
	h.poolLen++
	h.lastHandler = handler
}

// initInTarget 初始化入流量的录制目标
func (h *MessageHandler) initInTarget(pp MessagePoolParam) {
	pool := NewMessagePool(pp.MaxSize, pp.MessageExpire, pp.Handler)
	pool.MatchUUID(pp.TrackResponse)
	// listen address: ip+port
	pool.Address(pp.InTarget.Address)
	// set business protocol
	pool.Protocol(pp.InTarget.Protocol)
	pool.WithIsAspect(false)
	h.store(pp.InTarget.Address, pool)
}

// initInTarget 初始化出流量的录制目标
func (h *MessageHandler) initOutTarget(pp MessagePoolParam) {
	for _, target := range pp.OutTarget {
		pool := NewMessagePool(pp.MaxSize, pp.MessageExpire, pp.Handler)
		pool.MatchUUID(pp.TrackResponse)
		// listen address: ip+port
		pool.Address(target.Address)
		// set business protocol
		pool.Protocol(target.Protocol)
		pool.WithIsAspect(true)
		h.store(target.Address, pool)
	}
}

// Handler 处理每个 tcp 包
func (h *MessageHandler) Handler(packet gopacket.Packet) {
	pckt, err := ParsePacket(packet)
	if err != nil {
		logger.Debug3(fmt.Sprintf("error decoding packet(%dBytes):%s\n", packet.Metadata().CaptureLength, err))
		return
	}
	if pckt == nil {
		return
	}

	logger.Debug3(fmt.Sprintf("payload %s->%s seq:%d ack: %d, len: %d, pLen: %d, ts: %d, sum: %d, %s",
		pckt.Src(), pckt.Dst(), pckt.Seq, pckt.Ack, pckt.Length, len(pckt.Payload), pckt.Timestamp.Nanosecond(),
		pckt.Checksum, hex.EncodeToString(pckt.Payload)))
	if err = h.handlePack(pckt); err != nil {
		logger.Debug3("handle pack error: ", err)
	}
}

func (h *MessageHandler) handlePack(pack *Packet) error {
	pool, err := h.getPool(pack)
	if err != nil {
		return fmt.Errorf("not exits message pool with %s and %s", pack.Dst(), pack.Src())
	}

	pool.HandlePack(pack)
	return nil
}

func (h *MessageHandler) getPool(pack *Packet) (*MessagePool, error) {
	// if handler pool len = 1, just return that handler
	if h.poolLen == 1 {
		return h.lastHandler, nil
	}
	p, err := h.getIPPortPool(pack.Dst())
	if err != nil {
		return h.getIPPortPool(pack.Src())
	}

	return p, err
}

func (h *MessageHandler) getIPPortPool(ipPort string) (*MessagePool, error) {
	pIter, ok := h.pool.Load(ipPort)
	if !ok {
		return nil, errors.New("not exist pool")
	}

	p, ok := pIter.(*MessagePool)
	if !ok {
		return nil, errors.New("not *MessagePool")
	}

	return p, nil
}

// ParsePacket parse raw packets
func ParsePacket(packet gopacket.Packet) (*Packet, error) {
	// early check of error
	if packet == nil {
		return nil, errors.New("ignore nil packet")
	}

	// initialization
	pckt := new(Packet)
	pckt.Timestamp = packet.Metadata().Timestamp
	if pckt.Timestamp.IsZero() {
		pckt.Timestamp = time.Now()
	}

	// parsing link layer
	pckt.LinkLayer = packet.LinkLayer()

	const (
		ipV6 = 6
		ihl  = 40
	)
	// parsing network layer
	if net4, ok := packet.NetworkLayer().(*layers.IPv4); ok {
		pckt.Version = 4
		pckt.SrcIP = net4.SrcIP
		pckt.DstIP = net4.DstIP
		pckt.IHL = net4.IHL * 4
		pckt.Length = net4.Length
	} else if net6, ok := packet.NetworkLayer().(*layers.IPv6); ok {
		pckt.Version = ipV6
		pckt.SrcIP = net6.SrcIP
		pckt.DstIP = net6.DstIP
		pckt.IHL = ihl
		pckt.Length = net6.Length
	} else {
		pckt = nil
		return nil, errors.New("not IPv4 or IPv6")
	}

	// parsing tcp header(transportation layer)
	if tcp, ok := packet.TransportLayer().(*layers.TCP); ok {
		pckt.TCP = tcp
	} else {
		pckt = nil
		return nil, errors.New("not transportLayer")
	}

	pckt.DataOffset *= 4

	// calculating lost data
	headerSize := int(uint32(pckt.DataOffset) + uint32(pckt.IHL))
	if pckt.Version == ipV6 {
		headerSize -= ihl // in ipv6 the length of payload doesn't include the IPheader size
	}

	pckt.Lost = pckt.Length - uint16(headerSize+len(pckt.Payload))

	if packet.ErrorLayer() != nil {
		return nil, packet.ErrorLayer().Error()
	}

	return pckt, nil
}
