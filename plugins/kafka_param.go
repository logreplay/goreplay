package plugins

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/Shopify/sarama"
	"goreplay/config"
)

// crlf In HTTP newline defined by 2 bytes (for both windows and *nix support)
var crlf = []byte("\r\n")

// KafkaMessage should contains catched request information that should be
// passed as Json to Apache Kafka.
type KafkaMessage struct {
	ReqURL     string            `json:"Req_URL"`
	ReqType    string            `json:"Req_Type"`
	ReqID      string            `json:"Req_ID"`
	ReqTs      string            `json:"Req_Ts"`
	ReqMethod  string            `json:"Req_Method"`
	ReqBody    string            `json:"Req_Body,omitempty"`
	ReqHeaders map[string]string `json:"Req_Headers,omitempty"`
}

// NewTLSConfig loads TLS certificates
func NewTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	tlsConfig := tls.Config{}

	if clientCertFile != "" && clientKeyFile == "" {
		return &tlsConfig, errors.New("Missing key of client certificate in kafka")
	}
	if clientCertFile == "" && clientKeyFile != "" {
		return &tlsConfig, errors.New("missing TLS client certificate in kafka")
	}
	// Load client cert
	if (clientCertFile != "") && (clientKeyFile != "") {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return &tlsConfig, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	// Load CA cert
	if caCertFile != "" {
		caCert, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return &tlsConfig, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}
	return &tlsConfig, nil
}

// NewKafkaConfig returns Kafka config with or without TLS
func NewKafkaConfig(tlsConfig *config.KafkaTLSConfig) *sarama.Config {
	cfg := sarama.NewConfig()
	// Configuration options go here
	if tlsConfig != nil && (tlsConfig.ClientCert != "" || tlsConfig.CACert != "") {
		cfg.Net.TLS.Enable = true
		tlsConfig, err := NewTLSConfig(tlsConfig.ClientCert, tlsConfig.ClientKey, tlsConfig.CACert)
		if err != nil {
			log.Fatal(err)
		}
		cfg.Net.TLS.Config = tlsConfig
	}
	return cfg
}

// Dump returns the given request in its HTTP/1.x wire
// representation.
func (m KafkaMessage) Dump() ([]byte, error) {
	var b bytes.Buffer

	b.WriteString(fmt.Sprintf("%s %s %s\n", m.ReqType, m.ReqID, m.ReqTs))
	b.WriteString(fmt.Sprintf("%s %s HTTP/1.1", m.ReqMethod, m.ReqURL))
	b.Write(crlf)
	for key, value := range m.ReqHeaders {
		b.WriteString(fmt.Sprintf("%s: %s", key, value))
		b.Write(crlf)
	}

	b.Write(crlf)
	b.WriteString(m.ReqBody)

	return b.Bytes(), nil
}
