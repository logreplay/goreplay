package analyze

import (
	"fmt"

	"goreplay/codec"
	"goreplay/logreplay"
)

// CommonAnalyze 通用解析器
type CommonAnalyze interface {
	GetServiceCmd(data []byte) (string, error)
}

// Init 初始化解析器
func Init(protocol string) CommonAnalyze {
	protocolMap := map[string]CommonAnalyze{
		logreplay.ProtocolHTTP: &HTTP{},
		logreplay.ProtocolGrpc: &Grpc{},
	}
	if v, ok := protocolMap[protocol]; ok {
		return v
	}
	return nil
}

func getCmdByCodec(protocol string, data []byte) (string, error) {
	headerCodec := codec.GetHeaderCodec(protocol)
	if headerCodec == nil {
		return "", fmt.Errorf("getCmdByCodec not supported protocol: %s", protocol)
	}
	// 先解析请求头,注意这里解析的请求的 data
	ph, err := headerCodec.Decode(data, "")
	if err != nil {
		return "", err
	}
	return ph.APIName, nil
}
