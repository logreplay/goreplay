package codec

func init() {
	RegisterHeaderCodec("mysql", &mysqlHeaderCodecBuilder{})
}

type mysqlHeaderCodec struct {
}

// Decode decode buff data
func (e *mysqlHeaderCodec) Decode(_ []byte, _ string) (ProtocolHeader, error) {
	const sql = "SQL"
	protoHead := ProtocolHeader{
		ServiceName:   unknown,
		APIName:       sql,
		InterfaceName: sql,
		MethodName:    sql,
	}
	return protoHead, nil
}

type mysqlHeaderCodecBuilder struct {
}

// New 创建 mysqlHeaderCodec 的实例对象
func (builder *mysqlHeaderCodecBuilder) New() HeaderCodec {
	return &mysqlHeaderCodec{}
}
