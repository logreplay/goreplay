package tcp

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"time"
)

// 常量
const (
	bufferSize = 1000 * 20 // buffer 的初始化大小，这里只是简单的取经验值 2k
)

// Stats every message carry its own stats object
type Stats struct {
	LostData   int
	Length     int       // length of the data
	Start      time.Time // first packet's timestamp
	End        time.Time // last packet's timestamp
	SrcAddr    string
	DstAddr    string
	IsIncoming bool
	TimedOut   bool // timeout before getting the whole message
	Truncated  bool // last packet truncated due to max message size
	IPversion  byte
}

// Message is the representation of a tcp message
type Message struct {
	reqRspKey string // reqRspKey message request and response match key
	packets   []*Packet
	pool      *MessagePool
	buf       *bytes.Buffer
	feedback  interface{}
	Stats
	invalidFlag bool // invalid msg will be discarded
}

// NewMessage ...
func NewMessage(srcAddr, dstAddr string, ipVersion uint8, len int) (m *Message) {
	m = new(Message)
	m.DstAddr = dstAddr
	m.SrcAddr = srcAddr
	m.IPversion = ipVersion

	if len <= 0 {
		len = bufferSize
	}

	m.buf = bytes.NewBuffer(make([]byte, 0, len))
	return
}

// NewMessageForTest used for unit test
func NewMessageForTest(buf []byte) (m *Message) {
	m = new(Message)
	m.buf = bytes.NewBuffer(buf)
	return
}

// SetInvalid mark the msg as invalid
func (m *Message) SetInvalid() {
	m.invalidFlag = true
}

// Invalid indicate the msg invalid or not
func (m *Message) Invalid() bool {
	return m.invalidFlag
}

// Reset resets the message
func (m *Message) Reset() {
	if m.pool != nil {
		m.pool = nil
	}
}

// ReqRspKey return reqRspKey
func (m *Message) ReqRspKey() string {
	return m.reqRspKey
}

// SetReqRspKey set reqRspKey
func (m *Message) SetReqRspKey(key string) {
	m.reqRspKey = key
}

// Pool return pool
func (m *Message) Pool() *MessagePool {
	return m.pool
}

// UUID returns the UUID of a TCP request and its response.
func (m *Message) UUID() []byte {
	key := m.reqRspKey
	pckt := m.packets[0]

	// check if response or request have generated the ID before.
	if m.pool.uuidCache != nil {
		// if m.IsIncoming {
		// 	key = m.pool.framer.MessageKey(pckt, false).String()
		// } else {
		// 	key = m.pool.framer.MessageKey(pckt, true).String()
		// }

		if uuidHex, err := m.pool.uuidCache.Get([]byte(key)); err == nil {
			m.pool.uuidCache.Del([]byte(key))
			return uuidHex
		}
	}

	id := make([]byte, 12)
	binary.BigEndian.PutUint32(id, pckt.Seq)
	tStamp := m.End.UnixNano()
	binary.BigEndian.PutUint64(id[4:], uint64(tStamp))
	uuidHex := make([]byte, 24)
	hex.Encode(uuidHex[:], id[:])

	if m.pool.uuidCache != nil && m.IsIncoming {
		_ = m.pool.uuidCache.Set([]byte(key), uuidHex, int(m.pool.messageExpire.Seconds()))
	}

	return uuidHex
}

// ConnectionID returns the ID of a TCP connection.
func (m *Message) ConnectionID() string {
	return DefaultMessageKey(m.packets[0], false).String()
}

// Merge append message
func (m *Message) Merge(msg *Message) {
	for _, pckt := range msg.Packets() {
		m.add(pckt)
	}
}

// Add one packet to msg
func (m *Message) Add(pckt *Packet) {
	m.add(pckt)
}

func (m *Message) add(pckt *Packet) {
	m.Length += len(pckt.Payload)
	m.LostData += int(pckt.Lost)
	m.packets = append(m.packets, pckt)
	m.End = pckt.Timestamp
	m.buf.Write(pckt.Payload)
}

// Packets returns packets of the message
func (m *Message) Packets() []*Packet {
	return m.packets
}

// Data returns data in this message
func (m *Message) Data() []byte {
	return m.buf.Bytes()
}

// Truncate returns this message data length
func (m *Message) Truncate(n int) {
	m.buf.Truncate(n)
}

// SetFeedback set feedback/data that can be used later, e.g with End or Start hint
func (m *Message) SetFeedback(feedback interface{}) {
	m.feedback = feedback
}

// Feedback returns feedback associated to this message
func (m *Message) Feedback() interface{} {
	return m.feedback
}

// Sort a helper to sort packets
func (m *Message) Sort() {
	sort.SliceStable(m.packets, func(i, j int) bool { return m.packets[i].Seq < m.packets[j].Seq })
}

func _uint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
