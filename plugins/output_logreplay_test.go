package plugins

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"goreplay/config"
	"goreplay/logreplay"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/coocood/freecache"
	"github.com/stretchr/testify/suite"
)

const localhostGateway = "127.0.0.1:80"

// TestUnitLogreplay logreplay unit test execute
func TestUnitLogreplay(t *testing.T) {
	suite.Run(t, new(logreplaySuite))
}

type logreplaySuite struct {
	suite.Suite
	fakeErr error
}

// SetupTest which will run before each test in the suite.
func (s *logreplaySuite) SetupTest() {
	s.fakeErr = fmt.Errorf("fake error")
	// the overall mock sendRequest is mandatory, as it run in coroutine and consume channel all the time
	gomonkey.ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "sendRequest",
		func(*LogReplayOutput, string, *logreplay.GoReplayMessage) {})
}

func (s *logreplaySuite) TestPluginRead() {
	for _, tt := range []struct {
		name string
		data []byte
		mock func() *gomonkey.Patches
	}{
		{
			name: "success",
			data: []byte("example"),
			mock: func() *gomonkey.Patches {
				return gomonkey.
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "checkModuleAuth",
						func(*LogReplayOutput, *config.LogReplayOutputConfig) {})
			},
		},
	} {
		s.Run(tt.name, func() {
			patches := tt.mock()
			if patches != nil {
				defer patches.Reset()
			}

			output := NewLogReplayOutput("", &config.LogReplayOutputConfig{
				CommitID:            "1",
				APPID:               "1",
				ModuleID:            "1",
				APPKey:              "1",
				Protocol:            "gofree",
				TrackResponses:      true,
				Target:              "127.0.0.1:8000",
				ProtocolServiceName: "svc1",
			})

			go func() {
				output.(*LogReplayOutput).responses <- &response{payload: tt.data}
			}()

			msg, _ := output.PluginRead()
			s.Equal(msg.Data, tt.data)
		})
	}
}

func (s *logreplaySuite) TestPluginWrite() {
	for _, tt := range []struct {
		name string
		msg  *Message
		size int
		mock func() *gomonkey.Patches
	}{
		{
			name: "success",
			msg: &Message{
				Meta: []byte("111"),
				Data: []byte("1 1 1"),
			},
			size: 8,
			mock: func() *gomonkey.Patches {
				return gomonkey.
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "checkModuleAuth",
						func(*LogReplayOutput, *config.LogReplayOutputConfig) {})
			},
		},
		{
			name: "success2",
			msg: &Message{
				Meta: []byte("2 3\n3 3\n1"),
				Data: []byte("1 1 1"),
			},
			size: 14,
			mock: func() *gomonkey.Patches {
				return gomonkey.
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "checkModuleAuth",
						func(*LogReplayOutput, *config.LogReplayOutputConfig) {}).
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "parseResponse",
						func(*LogReplayOutput, *Message, []byte, string) (*logreplay.GoReplayMessage, error) { return nil, nil }).
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "sendRequest",
						func(*LogReplayOutput, string, *logreplay.GoReplayMessage) {}).
					ApplyMethod(reflect.TypeOf(&freecache.Cache{}), "Get",
						func(*freecache.Cache, []byte) (value []byte, err error) { return []byte("1"), nil })
			},
		},
		{
			name: "success3",
			msg: &Message{
				Meta: []byte{'1'},
				Data: []byte("1 1 1"),
			},
			size: 6,
			mock: func() *gomonkey.Patches {
				return gomonkey.
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "checkModuleAuth",
						func(*LogReplayOutput, *config.LogReplayOutputConfig) {}).
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "parseResponse",
						func(*LogReplayOutput, *Message, []byte, string) (*logreplay.GoReplayMessage, error) { return nil, nil }).
					ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "sendRequest",
						func(*LogReplayOutput, string, *logreplay.GoReplayMessage) {})
			},
		},
	} {
		s.Run(tt.name, func() {
			patches := tt.mock()
			if patches != nil {
				defer patches.Reset()
			}

			output := NewLogReplayOutput("", &config.LogReplayOutputConfig{
				CommitID:            "1",
				APPID:               "1",
				ModuleID:            "1",
				APPKey:              "1",
				Protocol:            "grpc",
				TrackResponses:      true,
				ProtocolServiceName: "svc1",
			})

			size, _ := output.PluginWrite(tt.msg)
			s.Equal(size, tt.size)
		})
	}
}

func (s *logreplaySuite) TestCheckOption() {
	for _, tt := range []struct {
		name      string
		conf      *config.LogReplayOutputConfig
		errorSign bool
	}{
		{
			name: "success",
			conf: &config.LogReplayOutputConfig{
				ModuleID:            "1",
				APPKey:              "1",
				APPID:               "1",
				Protocol:            "1",
				CommitID:            "1",
				Env:                 config.EnvTest,
				Timeout:             -1,
				Target:              "127.0.0.1:8080",
				ProtocolServiceName: "svc1",
			},
			errorSign: false,
		},
	} {
		s.Run(tt.name, func() {
			checkLogReplayConfig(tt.conf)
			if !tt.errorSign {
				s.Greater(tt.conf.QPSLimit, 0)
			}
		})
	}

}

func (s *logreplaySuite) TestParseResponse() {
	for _, tt := range []struct {
		name  string
		error error
		mock  func() *gomonkey.Patches
	}{
		{
			name: "success",
			mock: func() *gomonkey.Patches {
				return gomonkey.ApplyPrivateMethod(reflect.TypeOf(&LogReplayOutput{}), "checkModuleAuth",
					func(*LogReplayOutput, *config.LogReplayOutputConfig) {}).
					ApplyMethod(reflect.TypeOf(&freecache.Cache{}), "Get",
						func(*freecache.Cache, []byte) (value []byte, err error) { return []byte("{}"), nil })
			},
			error: nil,
		},
	} {
		s.Run(tt.name, func() {
			patches := tt.mock()
			if patches != nil {
				defer patches.Reset()
			}
			output := NewLogReplayOutput("", &config.LogReplayOutputConfig{
				ModuleID:            "1",
				APPKey:              "1",
				APPID:               "1",
				Protocol:            "grpc",
				CommitID:            "1",
				ProtocolServiceName: "svc1",
			})

			_, err := output.(*LogReplayOutput).parseResponse(&Message{}, []byte("a"), "")
			s.Equal(tt.error, err)
			_ = output.(*LogReplayOutput).Close()
		})
	}
}

func (s *logreplaySuite) TestRspUnmarshal() {
	s.Run("unmarshal_rsp", func() {
		// buf := []byte(`{"base_rsp":{"code":100000,"msg":"success"},"succeed":1}`)
		buf := []byte(`{"baseRsp":{"code":100000,"msg":"success"},"succeed":1}`)
		rsp := &logreplay.ReportRsp{}
		err := json.Unmarshal(buf, rsp)
		s.T().Logf("rsp: %+v; error: %v", rsp, err)
	})
}
