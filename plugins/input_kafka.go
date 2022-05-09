// Package plugins  插件实现
package plugins

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"goreplay/config"
	"goreplay/errors"
	"goreplay/logger"
	"goreplay/logreplay"
	"goreplay/protocol"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
)

const protocolKafka = "kafka"

// KafkaInput is used for receiving Kafka messages and
// transforming them into HTTP payloads.
type KafkaInput struct {
	config    *config.InputKafkaConfig
	consumers []sarama.PartitionConsumer
	messages  chan Message
	quit      chan struct{}
}

// NewKafkaInput creates instance of kafka consumer client with TLS config
func NewKafkaInput(address string, config *config.InputKafkaConfig, tlsConfig *config.KafkaTLSConfig) *KafkaInput {
	c := NewKafkaConfig(tlsConfig)
	var con sarama.Consumer
	if mock, ok := config.Consumer.(*mocks.Consumer); ok && mock != nil {
		con = config.Consumer
	} else {
		var err error
		con, err = sarama.NewConsumer(strings.Split(config.Host, ","), c)
		if err != nil {
			log.Fatalln("Failed to start Sarama(Kafka) consumer:", err)
		}
	}
	partitions, err := con.Partitions(config.Topic)
	if err != nil {
		log.Fatalln("failed to collect Sarama(Kafka) partitions:", err)
	}

	i := &KafkaInput{
		config:    config,
		consumers: make([]sarama.PartitionConsumer, len(partitions)),
		messages:  make(chan Message, 256),
		quit:      make(chan struct{}),
	}

	logger.Debug3("topic", config.Topic, " partitions: ", len(partitions))
	for index, partition := range partitions {
		// consume every partition
		consumer, err := con.ConsumePartition(config.Topic, partition, sarama.OffsetNewest)
		if err != nil {
			log.Fatalln("Failed to start Sarama(Kafka) partition consumer:", err)
		}

		go func(consumer sarama.PartitionConsumer) {
			defer consumer.Close()

			for message := range consumer.Messages() {
				req, err := parseReq(message, config.Host)
				if err != nil {
					logger.Error(err)
					continue
				}
				i.messages <- req
				rsp, err := parseRsp(message)
				if err != nil {
					logger.Error(err)
					continue
				}
				i.messages <- rsp
			}
		}(consumer)
		go i.ErrorHandler(consumer)
		i.consumers[index] = consumer
	}

	return i
}

// ErrorHandler should receive errors
func (i *KafkaInput) ErrorHandler(consumer sarama.PartitionConsumer) {
	for err := range consumer.Errors() {
		logger.Error("[INPUT-KAFKA] failed to read access log entry:", err)
	}
}

// parse ConsumerMessage to Message as request
func parseReq(message *sarama.ConsumerMessage, broker string) (Message, error) {
	var msg Message
	req := logreplay.KafkaConsumerReq{
		Topic:     message.Topic,
		Offset:    message.Offset,
		Partition: message.Partition,
		Broker:    broker,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}
	msg.Data = data
	msg.Meta = protocol.PayloadHeader(protocol.RequestPayload, message.Key, time.Now().UnixNano(), 0)
	return msg, nil
}

// parse ConsumerMessage to Message as response
func parseRsp(message *sarama.ConsumerMessage) (Message, error) {
	var msg Message
	data, err := json.Marshal(message)
	if err != nil {
		return Message{}, err
	}
	msg.Data = data
	msg.Meta = protocol.PayloadHeader(protocol.ResponsePayload, message.Key, time.Now().UnixNano(), 0)
	return msg, nil
}

// PluginRead a reads message from this plugin
func (i *KafkaInput) PluginRead() (*Message, error) {
	var msg Message
	select {
	case <-i.quit:
		return nil, errors.ErrorStopped
	case msg = <-i.messages:
	}

	msg.Protocol = protocolKafka

	return &msg, nil

}

// String toString函数
func (i *KafkaInput) String() string {
	return "Kafka Input: " + i.config.Host + "/" + i.config.Topic
}

// Close closes this plugin
func (i *KafkaInput) Close() error {
	close(i.quit)
	return nil
}
