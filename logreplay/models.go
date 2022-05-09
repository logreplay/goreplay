package logreplay

// ReportData 上报的数据结构
type ReportData struct {
	Batch []ReportItem `json:"batch"`
}

// ReportDataType 上报数据类型定义
type ReportDataType string

// BaseResp 基本响应结构体
type BaseResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// 上报数据类型常量
const (
	TypeAspect   ReportDataType = "aspect"
	TypeRecord   ReportDataType = "record"
	TypeReplay   ReportDataType = "replay"
	TypeCoverage ReportDataType = "coverage"
)

// ReportItem 数据上报项目
type ReportItem struct {
	Type ReportDataType `json:"type"`
	Data string         `json:"data"`
}

// ReportRsp 上报的响应结构体
// {"base_rsp":{"code":100000,"msg":"success"},"succeed":1}
type ReportRsp struct {
	BaseRsp *BaseRsp `json:"base_rsp"`
	Succeed int      `json:"succeed"`
}

// TaskRsp 创建任务的响应
type TaskRsp struct {
	TaskID uint32 `json:"task_id"`
}

// GoReplayReq goreplay 回放的请求结构体
type GoReplayReq struct {
	ModuleID       string `json:"module_id"`
	Operator       string `json:"operator"`
	Total          uint32 `json:"total"`
	Rate           uint32 `json:"rate"`
	RecordCommitID string `json:"record_commit_id"`
	Comment        string `json:"comment"`
	Addrs          string `json:"addrs"`
	ReplayType     uint32 `json:"replay_type"`
	TargetModuleID string `json:"target_module_id"`
	ReplayEnvTag   bool   `json:"replay_env_tag"`
}

// AuthRsp 授权
type AuthRsp struct {
	ID  string `json:"id"`
	KEY string `json:"key"`
}

// AuthReq 授权
type AuthReq struct {
	ModuleID string `json:"module_id"`
}

// GetModuleRsp 获取 module 响应体
type GetModuleRsp struct {
	Module Module `json:"module"`
}

// GetModuleReq 获取 module 请求体
type GetModuleReq struct {
	ModuleID string `json:"module_id"`
}

// Module 模块信息
type Module struct {
	AppID        string   `json:"app_id,omitempty"`
	ModuleID     string   `json:"module_id,omitempty"`
	ModuleNameEn string   `json:"module_name_en,omitempty"`
	AppNameEn    string   `json:"app_name_en,omitempty"`
	ModuleNameCh string   `json:"module_name_ch,omitempty"`
	Language     string   `json:"language,omitempty"`
	Creator      string   `json:"creator,omitempty"`
	CreateAt     float64  `json:"create_at,omitempty"`
	Owners       []string `json:"owners,omitempty"`
}

// ReportGoreplayStatusReq 上报 goreplay 状态 req
type ReportGoreplayStatusReq struct {
	IPAndPort string `json:"ip_port"`
}

// ReportGoreplayStatusRsp 上报 goreplay 状态 rsp
type ReportGoreplayStatusRsp struct {
	BaseRsp        *BaseRsp `json:"base_rsp"`
	GoreplayStatus string   `json:"goreplayStatus"`
}

// GetGoReplayTaskStatusReq 查询任务状态req
type GetGoReplayTaskStatusReq struct {
	TaskID string `json:"task_id"`
}

// GetGoReplayTaskStatusRsp 查询任务状态rsp
type GetGoReplayTaskStatusRsp struct {
	BaseRsp *BaseRsp `json:"base_rsp"`
	Status  int64    `json:"status"`
}

// UpdateGoReplayTaskRecordNumReq 更新录制数量req
type UpdateGoReplayTaskRecordNumReq struct {
	TaskID    string `json:"task_id"`
	RecordNum int32  `json:"record_num"`
}

// UpdateGoReplayTaskRecordNumRsp 更新录制数量rsp
type UpdateGoReplayTaskRecordNumRsp struct {
	BaseRsp   *BaseRsp `json:"base_rsp"`
	IsSuccess bool     `json:"is_success"`
}

// KafkaConsumerReq request
type KafkaConsumerReq struct {
	Broker    string
	Topic     string
	Offset    int64
	Partition int32
}

// ReportResp 上报的响应结构体
type ReportResp struct {
	BaseResp BaseResp `json:"base_resp"`
	Succeed  int      `json:"succeed"`
}
