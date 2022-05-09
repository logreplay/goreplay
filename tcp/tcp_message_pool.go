package tcp

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/google/gopacket"
	"goreplay/logger"
	"goreplay/size"
	"goreplay/tcp/ack"
)

// Handler message handler
type Handler func(*Message)

// HintEnd hints the pool to stop the session, see MessagePool.End
// when set, it will be executed before checking FIN or RST flag
type HintEnd func(*Message) bool

// HintStart hints the pool to start the reassembling the message, see MessagePool.Start
// when set, it will be called after checking SYN flag
type HintStart func(*Packet) (isIncoming, isOutgoing bool)

// MessagePool holds data of all tcp messages in progress(still receiving/sending packets).
// message is identified by its source port and dst port, and last 4bytes of src IP.
type MessagePool struct {
	sync.Mutex
	maxSize       size.Size // maximum message size, default 5mb
	pool          map[string]*Message
	uuidCache     *freecache.Cache
	handler       Handler
	messageExpire time.Duration // the maximum time to wait for the final packet, minimum is 100ms

	address        string // 录制服务地址，IP:PORT
	protocol       string
	framer         Framer
	longConnection bool
	isAspect       bool // 是否是切面流量
}

// NewMessagePool returns a new instance of message pool
func NewMessagePool(maxSize size.Size, messageExpire time.Duration, handler Handler) *MessagePool {
	pool := new(MessagePool)
	pool.handler = handler
	pool.messageExpire = time.Millisecond * 100
	if pool.messageExpire < messageExpire {
		pool.messageExpire = messageExpire
	}
	pool.maxSize = maxSize
	if pool.maxSize < 1 {
		pool.maxSize = 5 << 20
	}
	pool.pool = make(map[string]*Message)

	return pool
}

// MessageGroupBy group by all the pack
func (p *MessagePool) MessageGroupBy(packet *Packet) map[string]*Packet {
	if p.framer != nil {
		return p.framer.MessageGroupBy(packet)
	}

	return map[string]*Packet{p.MessageKey(packet, false).String(): packet}
}

// MessageKey generate key for msg
func (p *MessagePool) MessageKey(pckt *Packet, isPeer bool) *big.Int {
	if p.framer != nil {
		return p.framer.MessageKey(pckt, isPeer)
	}

	return DefaultMessageKey(pckt, isPeer)
}

// Pool return internal pool
func (p *MessagePool) Pool() map[string]*Message {
	return p.pool
}

func (p *MessagePool) needCacheServerAck(pckt *Packet) bool {
	return (p.longConnection && pckt.ACK && IsResponse(pckt, p.address) && len(pckt.Payload) > 0) ||
		(IsResponse(pckt, p.address) && pckt.SYN)
}

func (p *MessagePool) afterHandler(pckt *Packet) {
	key := DefaultMessageKey(pckt, false).String()
	// ack server -> client, exclude establish connection ack

	if p.needCacheServerAck(pckt) {
		// update server ack after every response, it will be next request seq
		ack.PutServerAck(key, pckt.Ack)

		logger.Debug3(fmt.Sprintf("cache response ack: %s -> %s, key: %s, ack: %d",
			pckt.Src(), pckt.Dst(), key, pckt.Ack), pckt.Flag())
	}
}

// Handler returns packet handler
func (p *MessagePool) Handler(packet gopacket.Packet) {
	pckt, err := p.ParsePacket(packet)
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

	p.HandlePack(pckt)
}

// HandlePack 处理单个packet
func (p *MessagePool) HandlePack(pckt *Packet) {
	var in, out bool
	p.Lock()
	defer p.Unlock()

	defer func(pack *Packet) {
		p.afterHandler(pack)
	}(pckt)

	packsMap := p.MessageGroupBy(pckt)
	if packsMap == nil {
		logger.Debug3(fmt.Sprintf("packsMap: %v", packsMap))
		return
	}

	for key, itemPckt := range packsMap {
		m, ok := p.pool[key]
		logger.Debug3("found in pool: ", ok, ",key: ", key)
		if itemPckt.RST {
			if ok {
				p.dispatch(key, m)
			}

			key = p.MessageKey(itemPckt, true).String()

			m, ok = p.pool[key]
			if ok {
				p.dispatch(key, m)
			}
			go logger.Debug3(fmt.Sprintf("RST flag from %s to %s at %s\n",
				itemPckt.Src(), itemPckt.Dst(), itemPckt.Timestamp))
			continue
		}

		switch {
		case ok:
			p.addPacket(key, m, itemPckt)
			continue
		case itemPckt.SYN:
			in = !itemPckt.ACK
		case p.framer != nil:
			if in, out = p.framer.Start(itemPckt); !(in || out) {
				continue
			}

		default:
			continue
		}

		// 指定buffer的初始化大小, 减少扩容次数, 这里无法知道最终的buffer应该多大, payload的3倍只是经验值, 不一定适合所有场景
		m = NewMessage(itemPckt.Src(), itemPckt.Dst(), itemPckt.Version, 3*len(itemPckt.Payload))
		m.IsIncoming = in
		if p.framer == nil {
			// response get peer's key(request key)
			m.reqRspKey = DefaultMessageKey(pckt, !in).String()
		} else {
			m.reqRspKey = p.framer.ReqRspKey(pckt)
		}

		logger.Debug3("first addPacket message:", key, itemPckt.Src(), itemPckt.Dst(), itemPckt.Flag(), m.reqRspKey)

		p.pool[key] = m
		m.Start = itemPckt.Timestamp
		m.pool = p
		p.addPacket(key, m, itemPckt)
	}
}

// MatchUUID instructs the pool to use same UUID for request and responses
// this function should be called at initial stage of the pool
func (p *MessagePool) MatchUUID(match bool) {
	if match {
		// 20M
		p.uuidCache = freecache.NewCache(20 * 1024 * 1024)
		return
	}
	p.uuidCache = nil
}

// Dispatch send msg to processor
func (p *MessagePool) Dispatch(key string, m *Message) {
	p.dispatch(key, m)
}

func (p *MessagePool) dispatch(key string, m *Message) {
	delete(p.pool, key)
	p.handler(m)
}

func (p *MessagePool) shouldDispatch(m *Message) bool {
	if m == nil {
		return false
	}

	return time.Since(m.Start) > p.messageExpire
}

func (p *MessagePool) addPacket(key string, m *Message, pckt *Packet) {
	trunc := m.Length + len(pckt.Payload) - int(p.maxSize)
	if trunc > 0 {
		m.Truncated = true
		pckt.Payload = pckt.Payload[:int(p.maxSize)-m.Length]
	}

	logger.Debug3("addPacket key:", key, pckt.Src(), pckt.Dst(), pckt.Flag())

	m.add(pckt)

	switch {
	// if one of this cases matches, we dispatch the message
	case trunc >= 0:
	case pckt.FIN:
	case p.framer != nil && p.framer.End(m):
		// ack client -> server, exclude disconnect ack
		logger.Debug3(fmt.Sprintf("end of %s -> %s key: %s, flag: %s", pckt.Src(), pckt.Dst(), key, pckt.Flag()))
		if p.longConnection && IsRequest(pckt, p.address) && !pckt.FIN {
			// update client ack after every request when request end, it is this request's response start seq
			cacheKey := DefaultMessageKey(pckt, false).String()
			ack.PutClientAck(cacheKey, pckt.Ack)

			logger.Debug3(fmt.Sprintf("cache request ack: %s -> %s, key: %s, ack: %d, cacheKey: %s",
				pckt.Src(), pckt.Dst(), key, pckt.Ack, cacheKey), pckt.Flag())
		}
	case p.shouldDispatch(m):
		// avoid dispatching message twice
		if _, ok := p.pool[key]; !ok {
			return
		}
		m.TimedOut = true
	default:
		// continue to receive packets
		logger.Debug3(fmt.Sprintf("default continue to receive packets, key %s m.packets length: %d",
			key, len(m.Packets())))
		return
	}

	logger.Debug3("addPacket -> dispatch", pckt.Src(), pckt.Dst(), pckt.Flag(), "Packets", len(m.Packets()),
		hex.EncodeToString(m.buf.Bytes()))

	p.dispatch(key, m)
}

// Address listen destination address
func (p *MessagePool) Address(address string) {
	p.address = address
}

// Protocol record business protocol
func (p *MessagePool) Protocol(protocol string) {
	if protocol == "" {
		log.Fatal("please specify input-raw-protocol")
	}
	p.protocol = protocol
	// get record protocol frame builder
	fb := GetFramerBuilder(protocol)
	if fb != nil {
		p.longConnection = true
		p.framer = fb.New(p.address)
	}

	logger.Debug3("longConnection:", p.longConnection, protocol)
}

// GetProtocol get protocol of message pool
func (p *MessagePool) GetProtocol() string {
	return p.protocol
}

// WithIsAspect 设置 isAspect
func (p *MessagePool) WithIsAspect(isAspect bool) {
	p.isAspect = isAspect
}
