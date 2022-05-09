// Package emitter 分发器
package emitter

import (
	"fmt"
	"io"
	"sync"
	"time"

	"goreplay/byteutils"
	"goreplay/config"
	"goreplay/errors"
	"goreplay/http"
	"goreplay/logger"
	"goreplay/plugins"
	"goreplay/plugins/middleware"
	"goreplay/protocol"
	"goreplay/size"
)

// Settings emitter设置
type Settings struct {
	CopyBufferSize size.Size
	PrettifyHTTP   bool
	Split          bool
	ModifierConfig config.HTTPModifierConfig
}

// Emitter represents an abject to manage plugins communication
type Emitter struct {
	sync.WaitGroup
	inOutPlugins *plugins.InOutPlugins
	settings     Settings
}

// NewEmitter creates and initializes new Emitter object.
func NewEmitter(settings Settings) *Emitter {
	return &Emitter{
		settings: settings,
	}
}

// Start initialize loop for sending data from inputs to outputs
func (e *Emitter) Start(inOutPlugins *plugins.InOutPlugins, middlewareCmd string) {
	if e.settings.CopyBufferSize < 1 {
		e.settings.CopyBufferSize = 5 << 20
	}

	e.inOutPlugins = inOutPlugins

	if middlewareCmd != "" {
		midWare := middleware.NewMiddleware(middlewareCmd)
		for _, in := range inOutPlugins.Inputs {
			midWare.ReadFrom(in)
		}

		e.inOutPlugins.Inputs = append(e.inOutPlugins.Inputs, midWare)
		e.inOutPlugins.All = append(e.inOutPlugins.All, midWare)
		e.Add(1)
		go func() {
			defer e.Done()
			e.copyMulty(midWare, inOutPlugins.Outputs...)
		}()

		return
	}

	for _, in := range inOutPlugins.Inputs {
		e.Add(1)
		go func(in plugins.PluginReader) {
			defer e.Done()
			e.copyMulty(in, inOutPlugins.Outputs...)
		}(in)
	}

}

// Close closes all the goroutine and waits for it to finish.
func (e *Emitter) Close() {
	for _, p := range e.inOutPlugins.All {
		if cp, ok := p.(io.Closer); ok {
			_ = cp.Close()
		}
	}

	if len(e.inOutPlugins.All) > 0 {
		// wait for everything to stop
		e.Wait()
	}

	e.inOutPlugins.All = nil // avoid Close to make changes again
}

// copyMulty copies from 1 reader to multiple writers
func (e *Emitter) copyMulty(src plugins.PluginReader, writers ...plugins.PluginWriter) {
	var ok bool
	wIndex := 0
	modifier := http.NewHTTPModifier(&e.settings.ModifierConfig)
	filteredRequests := make(map[string]int64)
	filteredRequestsLastCleanTime := time.Now().UnixNano()
	filteredCount := 0

	for {
		msg, err := src.PluginRead()
		if err != nil {
			if err == errors.ErrorFilterFromIP ||
				err == errors.ErrorFilterCmd ||
				err == errors.ErrorFilterMsgType {
				continue
			}

			// 判断错误，若为ErrorStopped和EOF之外的错误则返回error，否则返回Nil
			if ignoreError(err) {
				return
			}

			logger.Debug2(fmt.Sprintf("[EMITTER] error during copy: %q", err))

			return
		}

		if checkMsg(msg) {
			if len(msg.Data) > int(e.settings.CopyBufferSize) {
				logger.Debug2(fmt.Sprintf("[EMITTER] len(msg.Data) = %d > %d",
					len(msg.Data), int(e.settings.CopyBufferSize)))
				msg.Data = msg.Data[:e.settings.CopyBufferSize]
			}

			if filteredRequests, ok = e.prettify(modifier, msg, src, filteredRequests, &filteredCount); !ok {
				continue
			}

			if err := e.splitOutput(&writers, &wIndex, msg); err != nil {
				logger.Debug2(fmt.Sprintf("[EMITTER] error during copy: %q", err))
				return
			}
		}

		if checkFilteredCount(filteredCount) {
			continue
		}
		// Run GC on each 1000 request
		filteredRequests = e.garbageCollect(filteredRequests, &filteredRequestsLastCleanTime, &filteredCount)
	}
}

// garbageCollect Clean up filtered requests for which we didn't get a response to filter
func (e *Emitter) garbageCollect(requests map[string]int64, lastCleanTime *int64, count *int) map[string]int64 {
	now := time.Now().UnixNano()
	if now-*lastCleanTime <= int64(60*time.Second) {
		return requests
	}

	for k, v := range requests {
		if now-v > int64(60*time.Second) {
			delete(requests, k)
			*count--
		}
	}

	*lastCleanTime = time.Now().UnixNano()

	return requests
}

// splitOutput 将msg写入writer中
func (e *Emitter) splitOutput(writers *[]plugins.PluginWriter,
	index *int, msg *plugins.Message) error {
	if e.settings.Split {
		// Simple round robin
		if _, err := (*writers)[*index].PluginWrite(msg); err != nil {
			return err
		}

		*index = (*index + 1) % len(*writers)
	} else {
		for _, dst := range *writers {
			if _, err := dst.PluginWrite(msg); err != nil {
				logger.Error(fmt.Sprintf("writers %T, err: %v", dst, err))

				return err
			}
		}
	}

	return nil
}

// ignoreError 判断错误类型,若为ErrorStopped和EOF则跳过
func ignoreError(err error) bool {
	if err == errors.ErrorStopped || err == io.EOF {
		return true
	}

	return false
}

// checkMsg 判断Msg是否有效
func checkMsg(msg *plugins.Message) bool {
	if msg != nil && len(msg.Data) > 0 {
		return true
	}

	return false
}

// checkFilteredCount 判断计数
func checkFilteredCount(count int) bool {
	if count <= 0 || count%1000 != 0 {
		return true
	}

	return false
}

// prettify prettifyHTTP and rewrite msg
func (e *Emitter) prettify(modifier *http.Modifier, msg *plugins.Message,
	src plugins.PluginReader, requestsMap map[string]int64, count *int) (map[string]int64, bool) {
	meta := protocol.PayloadMeta(msg.Meta)
	if len(meta) < 3 {
		logger.Debug2(fmt.Sprintf("[EMITTER] Found malformed record %q from %q", msg.Meta, src))

		return requestsMap, false
	}

	requestID := byteutils.SliceToString(meta[1])
	// start a subroutine only when necessary
	logger.Debug3("[EMITTER] input: ", byteutils.SliceToString(msg.Meta[:len(msg.Meta)-1]), " from: ", src)
	if modifier != nil {
		logger.Debug3("[EMITTER] modifier:", requestID, "from:", src)
		if protocol.IsRequestPayload(msg.Meta) {
			msg.Data = modifier.Rewrite(msg.Data)
			// If modifier tells to skip request
			if len(msg.Data) == 0 {
				requestsMap[requestID] = time.Now().UnixNano()
				*count++

				return requestsMap, false
			}

			logger.Debug("[EMITTER] Rewritten input:", requestID, "from:", src)

		} else {
			if _, ok := requestsMap[requestID]; ok {
				delete(requestsMap, requestID)
				*count--

				return requestsMap, false
			}
		}
	}

	if e.settings.PrettifyHTTP {
		msg.Data = http.PrettifyHTTP(msg.Data)
		if len(msg.Data) == 0 {
			return requestsMap, false
		}
	}

	return requestsMap, true
}
