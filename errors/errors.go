package errors

import "errors"

// ErrorStopped is the error returned when the go routines reading the input is stopped.
var ErrorStopped = errors.New("reading stopped")

// ErrorFilterFromIP is the error returned when filter packets based on IP
var ErrorFilterFromIP = errors.New("filter packets based on ip")

// ErrorFilterMsgType 录制流量数据类型过滤 req 请求  rsp 响应
var ErrorFilterMsgType = errors.New("filter msg type not match")

// ErrorFilterCmd 录制流量命令字过滤
var ErrorFilterCmd = errors.New("filter cmd not match")
