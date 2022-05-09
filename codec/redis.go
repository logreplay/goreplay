package codec

func init() {
	RegisterHeaderCodec("redis", &redisHeaderCodecBuilder{})
}

type redisHeaderCodec struct {
}

// Decode decode buff data
func (e *redisHeaderCodec) Decode(_ []byte, _ string) (ProtocolHeader, error) {
	const redis = "REDIS"
	protoHead := ProtocolHeader{
		ServiceName:   unknown,
		APIName:       redis,
		InterfaceName: redis,
		MethodName:    redis,
	}
	return protoHead, nil
}

type redisHeaderCodecBuilder struct {
}

// New 创建 redisHeaderCodec 的实例对象
func (builder *redisHeaderCodecBuilder) New() HeaderCodec {
	return &redisHeaderCodec{}
}
