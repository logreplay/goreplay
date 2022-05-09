package config

import (
	"fmt"
	"time"

	"github.com/Shopify/sarama"
	"goreplay/size"
)

// Available engines for intercepting traffic
const (
	EnginePcap EngineType = 1 << iota
	EnginePcapFile
	EngineRawSocket

	lp = "libpcap"
	pf = "pcap_file"
	rs = "raw_socket"
	af = "af_packet"
)

// RAWInputConfig represents configuration that can be applied on raw input
type RAWInputConfig struct {
	PcapOptions
	Expire              time.Duration `json:"input-raw-expire"`
	CopyBufferSize      size.Size     `json:"copy-buffer-size"`
	Engine              EngineType    `json:"input-raw-engine"`
	TrackResponse       bool          `json:"input-raw-track-response"`
	Protocol            string        `json:"input-raw-protocol"`
	RecordMsgType       string        `json:"input-raw-record-msg-type"`
	RecordMsgCmd        string        `json:"input-raw-record-msg-cmd"`
	RealIPHeader        string        `json:"input-raw-realip-header"`
	Stats               bool          `json:"input-raw-stats"`
	Logreplay           bool          `json:"input-raw-logreplay"`
	LogreplaySampleRate int           `json:"input-raw-logreplay-sample-rate"`
	AutoSelectIP        bool          `json:"input-raw-auto-select-ip"` // 自动选择Ip
	SelectHost          string        `json:"input-raw-select-host"`    // 录制指定host的流量, 如果指定多个host来源。使用 "," 进行分割
	AspectInfo          string        `json:"input-raw-aspect-info"`    // 录制切面流量, 如"127.0.0.1:3306:mysql,
	// 127.0.0.1:1234:redis"
	Quit chan bool // Channel used only to indicate goroutine should shutdown
	Host string
	Port uint16
}

// TCPInputConfig represents configuration of a TCP input plugin
type TCPInputConfig struct {
	Secure          bool   `json:"input-tcp-secure"`
	CertificatePath string `json:"input-tcp-certificate"`
	KeyPath         string `json:"input-tcp-certificate-key"`
}

// UDPInputConfig represents configuration of a UDP input plugin
type UDPInputConfig struct {
	TrackResponse bool   `json:"input-udp-track-response"`
	Protocol      string `json:"input-udp-protocol"`
}

// PcapOptions options that can be set on a pcap capture handle,
// these options take effect on inactive pcap handles
type PcapOptions struct {
	BufferTimeout time.Duration `json:"input-raw-buffer-timeout"`
	TimestampType string        `json:"input-raw-timestamp-type"`
	BPFFilter     string        `json:"input-raw-bpf-filter"`
	BufferSize    size.Size     `json:"input-raw-buffer-size"`
	Promiscuous   bool          `json:"input-raw-promisc"`
	Monitor       bool          `json:"input-raw-monitor"`
	Snaplen       bool          `json:"input-raw-override-snaplen"`
}

// InputKafkaConfig should contains required information to build producers.
type InputKafkaConfig struct {
	Consumer sarama.Consumer
	Host     string `json:"input-kafka-host"`
	Topic    string `json:"input-kafka-topic"`
	UseJSON  bool   `json:"input-kafka-json-format"`
}

// KafkaTLSConfig should contains TLS certificates for connecting to secured Kafka clusters
type KafkaTLSConfig struct {
	CACert     string `json:"kafka-tls-ca-cert"`
	ClientCert string `json:"kafka-tls-client-cert"`
	ClientKey  string `json:"kafka-tls-client-key"`
}

// EngineType define engine type
type EngineType uint8

// Set is here so that EngineType can implement flag.Var
func (eng *EngineType) Set(v string) error {
	switch v {
	case "", lp:
		*eng = EnginePcap
	case pf:
		*eng = EnginePcapFile
	case rs, af:
		*eng = EngineRawSocket
	default:
		return fmt.Errorf("invalid engine %s", v)
	}
	return nil
}

// String engine type string
func (eng *EngineType) String() (e string) {
	switch *eng {
	case EnginePcapFile:
		e = pf
	case EnginePcap:
		e = lp
	case EngineRawSocket:
		e = rs
	default:
		e = ""
	}
	return e
}
