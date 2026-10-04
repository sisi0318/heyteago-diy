// Package domain 承载杯贴工具的纯领域类型与规则，不依赖任何外层。
package domain

import "encoding/json"

// 喜茶杯贴画布规格，来自官方 App 内嵌代码。
const (
	CupWidth  = 596
	CupHeight = 832

	// MaxUploadBytes 是服务端对上传文件的兜底上限（各平台共用）。
	// 前端渲染管线按平台上限压缩产物（喜茶 200KB），这里仅防越界请求。
	MaxUploadBytes = 2 << 20
)

// Result 是平台业务响应的统一形状（code/message/data），喜茶 App 与奈雪小程序通道一致。
type Result struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
