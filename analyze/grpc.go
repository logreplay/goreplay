package analyze

import "goreplay/codec"

// Grpc 解析器
type Grpc struct {
}

// GetServiceCmd 获取命令字
func (g *Grpc) GetServiceCmd(data []byte) (string, error) {
	return getCmdByCodec(codec.GrpcName, data)
}
