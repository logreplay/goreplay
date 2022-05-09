package framer

import (
	"bytes"
	"fmt"

	"github.com/golang/groupcache/lru"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

var (
	cache                    = lru.New(65535)
	http2InitHeaderTableSize = uint32(4096)
	readMetaHeadersKey       = "ReadMetaHeaders_%s"
)

const (
	// LogReplayTraceID logreplay 的 traceID
	LogReplayTraceID = "_log_replay_trace_id"
)

// HTTP2Framer 自定义 Http2Framer
type HTTP2Framer struct {
	*http2.Framer
	payload   []byte
	readIndex int
}

// NewHTTP2Framer news a HTTP2Framer
func NewHTTP2Framer(payload []byte, cacheKey string, useNewDecoder bool) *HTTP2Framer {
	fr := http2.NewFramer(nil, bytes.NewReader(payload))

	if useNewDecoder {
		fr.ReadMetaHeaders = hpack.NewDecoder(http2InitHeaderTableSize, nil)
		return &HTTP2Framer{Framer: fr, payload: payload}
	}

	if cacheKey == "" {
		return &HTTP2Framer{Framer: fr, payload: payload}
	}

	cacheKey = fmt.Sprintf(readMetaHeadersKey, cacheKey)

	cacheDecoder, ok := cache.Get(cacheKey)
	if ok {
		// cache 中存的对象必定是*hpack.Decoder
		fr.ReadMetaHeaders, _ = cacheDecoder.(*hpack.Decoder)
	} else {
		decoder := hpack.NewDecoder(http2InitHeaderTableSize, nil)
		cache.Add(cacheKey, decoder)
		fr.ReadMetaHeaders = decoder
	}

	return &HTTP2Framer{Framer: fr, payload: payload}
}

// ReadFrameAndBytes 读取一个 frame, 包括它的原始二进制数据
func (fr *HTTP2Framer) ReadFrameAndBytes() ([]byte, http2.Frame, error) {
	f, err := fr.ReadFrame()
	if err != nil {
		return nil, nil, err
	}

	eIndex := fr.readIndex + int(f.Header().Length) + 9
	pl := fr.payload[fr.readIndex:eIndex]

	defer func() {
		fr.readIndex = eIndex
	}()

	return pl, f, nil
}

// ReEncodeMetaHeadersFrame 重新 encode http2 的 headers
func ReEncodeMetaHeadersFrame(header *http2.MetaHeadersFrame) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)

	for _, hf := range header.Fields {
		err := enc.WriteField(hf)
		if err != nil {
			return nil, err
		}
	}

	traceIDHeader := hpack.HeaderField{
		Name:  LogReplayTraceID,
		Value: uuid.NewString(),
	}

	err := enc.WriteField(traceIDHeader)
	if err != nil {
		return nil, err
	}

	hfp := http2.HeadersFrameParam{
		StreamID:      header.StreamID,
		BlockFragment: buf.Bytes(),
		EndStream:     header.StreamEnded(),
		EndHeaders:    header.HeadersEnded(),
	}

	var dstBuf bytes.Buffer
	nfr := http2.NewFramer(&dstBuf, nil)
	err = nfr.WriteHeaders(hfp)
	if err != nil {
		return nil, err
	}

	return dstBuf.Bytes(), nil
}
