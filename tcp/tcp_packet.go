package tcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

/*
Packet represent data and layers of packet.
parser extracts information from pcap Packet. functions of *Packet doesn't validate if packet is nil,
calllers must make sure that ParsePacket has'nt returned any error before calling any other
function.
*/
type Packet struct {
	// Link layer
	gopacket.LinkLayer

	// IP Header
	Version uint8 // Ip version
	SrcIP   net.IP
	DstIP   net.IP
	IHL     uint8
	Length  uint16

	// TCP Segment Header
	*layers.TCP

	// Data info
	Lost      uint16
	Timestamp time.Time
}

// Copy copys a packet
func (pckt *Packet) Copy() *Packet {
	cp := new(Packet)
	cp.LinkLayer = pckt.LinkLayer
	cp.Version = pckt.Version
	cp.SrcIP = pckt.SrcIP
	cp.DstIP = pckt.DstIP
	cp.IHL = pckt.IHL
	cp.Length = pckt.Length
	cp.TCP = pckt.TCP
	cp.Lost = pckt.Lost
	cp.Timestamp = pckt.Timestamp

	cp.Payload = []byte{}

	return cp
}

// ParsePacket parse raw packets
func (p *MessagePool) ParsePacket(packet gopacket.Packet) (*Packet, error) {
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

	// parsing network layer
	if net4, ok := packet.NetworkLayer().(*layers.IPv4); ok {
		pckt.Version = 4
		pckt.SrcIP = net4.SrcIP
		pckt.DstIP = net4.DstIP
		pckt.IHL = net4.IHL * 4
		pckt.Length = net4.Length
	} else if net6, ok := packet.NetworkLayer().(*layers.IPv6); ok {
		pckt.Version = 6
		pckt.SrcIP = net6.SrcIP
		pckt.DstIP = net6.DstIP
		pckt.IHL = 40
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
	if pckt.Version == 6 {
		headerSize -= 40 // in ipv6 the length of payload doesn't include the IPheader size
	}

	pckt.Lost = pckt.Length - uint16(headerSize+len(pckt.Payload))

	if packet.ErrorLayer() != nil {
		return nil, packet.ErrorLayer().Error()
	}

	return pckt, nil
}

// Src returns the source socket of a packet
func (pckt *Packet) Src() string {
	return fmt.Sprintf("%s:%d", pckt.SrcIP, pckt.SrcPort)
}

// Dst returns destination socket
func (pckt *Packet) Dst() string {
	return fmt.Sprintf("%s:%d", pckt.DstIP, pckt.DstPort)
}

// SYNOptions returns MSS and windowscale of syn packets
func (pckt *Packet) SYNOptions() (mss uint16, windowscale byte) {
	if !pckt.SYN {
		return
	}
	for _, v := range pckt.Options {
		if v.OptionType == layers.TCPOptionKindMSS {
			mss = binary.BigEndian.Uint16(v.OptionData)
			continue
		}
		if v.OptionType == layers.TCPOptionKindWindowScale {
			if v.OptionLength > 0 {
				windowscale = v.OptionData[0]
			}
		}
	}
	return
}

// LinkInfo returns info about the link layer
func (pckt *Packet) LinkInfo() string {
	if l, ok := pckt.LinkLayer.(*layers.Ethernet); ok {
		return fmt.Sprintf(
			"Source Mac: %s\nDestination Mac: %s\nProtocol: %s",
			l.SrcMAC,
			l.DstMAC,
			l.EthernetType,
		)
	}
	return "<Not Ethernet>"
}

// Flag returns formatted tcp flags
func (pckt *Packet) Flag() (flag string) {
	if pckt.FIN {
		flag += "FIN, "
	}
	if pckt.SYN {
		flag += "SYN, "
	}
	if pckt.RST {
		flag += "RST, "
	}
	if pckt.PSH {
		flag += "PSH, "
	}
	if pckt.ACK {
		flag += "ACK, "
	}
	if pckt.URG {
		flag += "URG, "
	}
	if len(flag) != 0 {
		return flag[:len(flag)-2]
	}
	return flag
}

// String output for a TCP Packet
func (pckt *Packet) String() string {
	return fmt.Sprintf(`Time: %s
%s
Source: %s
Destination: %s
IHL: %d
Total Length: %d
Sequence: %d
Acknowledgment: %d
DataOffset: %d
Window: %d
Flag: %s
Options: %s
Data Size: %d
Lost Data: %d`,
		pckt.Timestamp.Format(time.StampNano),
		pckt.LinkInfo(),
		pckt.Src(),
		pckt.Dst(),
		pckt.IHL,
		pckt.Length,
		pckt.Seq,
		pckt.Ack,
		pckt.DataOffset,
		pckt.Window,
		pckt.Flag(),
		pckt.Options,
		len(pckt.Payload),
		pckt.Lost,
	)
}
