package usecase

import (
	"errors"
	"fmt"
)

// 入参校验错误，transport 层映射为 400。
var (
	ErrMissingToken   = errors.New("缺少授权 token")
	ErrInvalidToken   = errors.New("token 格式不正确")
	ErrTokenExpired   = errors.New("token 已过期")
	ErrMissingUserID  = errors.New("缺少用户 ID")
	ErrMissingFile    = errors.New("缺少文件")
	ErrFileTooLarge   = errors.New("文件超过大小上限")
	ErrInvalidPhone   = errors.New("手机号格式不正确")
	ErrMissingSmsCode = errors.New("缺少短信验证码")
	ErrMissingTicket  = errors.New("缺少人机验证 ticket")
)

// BusinessError 表示平台上游返回了非 0 业务码。
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("上游业务错误 code=%d: %s", e.Code, e.Message)
}

// SignError 表示本地签名失败，transport 层映射为 502。
type SignError struct {
	Err error
}

func (e *SignError) Error() string { return "签名失败：" + e.Err.Error() }
func (e *SignError) Unwrap() error { return e.Err }

// UpstreamError 表示请求平台失败（网络错误、HTTP 非 2xx、响应无法解析），
// transport 层映射为 502。
type UpstreamError struct {
	Err error
}

func (e *UpstreamError) Error() string { return e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }
