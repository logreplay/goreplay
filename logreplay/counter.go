package logreplay

import "sync/atomic"

// Counter 计数器
type Counter struct {
	recordNum uint32 // 已上报的总数
}

// NewCounter 初始化
func NewCounter() *Counter {
	return &Counter{}
}

// RecordNum return the record num
func (c *Counter) RecordNum() uint32 {
	return atomic.LoadUint32(&c.recordNum)
}

// IncrRecordNum add recordNum and return new
func (c *Counter) IncrRecordNum(cnt uint32) uint32 {
	return atomic.AddUint32(&c.recordNum, cnt)
}
