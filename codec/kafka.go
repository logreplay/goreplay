// Package codec codec for kafka
package codec

import (
	"encoding/json"
	"goreplay/logreplay"
)

func init() {
	RegisterHeaderCodec("kafka", &kafkaHeaderCodecBuilder{})
}

type kafkaHeaderCodec struct {
}
type kafkaHeaderCodecBuilder struct {
}

// Decode decode buff data
func (e *kafkaHeaderCodec) Decode(data []byte, _ string) (ProtocolHeader, error) {
	var kr logreplay.KafkaConsumerReq
	if err := json.Unmarshal(data, &kr); err != nil {
		return ProtocolHeader{}, err
	}
	const kafka = "kafka"
	protoHead := ProtocolHeader{
		ServiceName:   kr.Broker,
		APIName:       kr.Topic,
		InterfaceName: kafka,
		MethodName:    kafka,
	}
	return protoHead, nil
}

// New 创建 kafkaHeaderCodec 的实例对象
func (builder *kafkaHeaderCodecBuilder) New() HeaderCodec {
	return &kafkaHeaderCodec{}
}
