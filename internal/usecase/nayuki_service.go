package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DiheMoe/heyteago-diy/internal/domain"
)

// cst 用于展示 token 到期时间（服务端容器时区为 UTC）。
var cst = time.FixedZone("CST", 8*3600)

// NayukiService 承载奈雪杯贴上传（小程序通道）：图片经 OSS 直传后提交为杯贴作品。
// 小程序登录依赖微信授权，无法在本机复现，token 由用户抓包粘贴。
type NayukiService struct {
	gateway NayukiGateway
	now     func() time.Time // 测试注入固定时钟
}

func NewNayukiService(gateway NayukiGateway) *NayukiService {
	return &NayukiService{gateway: gateway, now: time.Now}
}

// Upload 上传杯贴：校验 → OSS 直传 → 提交作品。
func (s *NayukiService) Upload(ctx context.Context, in StickerUpload) (UploadOutput, error) {
	if in.Token == "" {
		return UploadOutput{}, ErrMissingToken
	}
	if err := validateFile(in.File); err != nil {
		return UploadOutput{}, err
	}

	imageURL, err := s.gateway.UploadImage(ctx, in.Token, in.ContentType, in.File)
	if err != nil {
		return UploadOutput{}, err
	}
	data, err := s.gateway.SaveWork(ctx, in.Token, imageURL)
	if err != nil {
		return UploadOutput{}, err
	}
	return UploadOutput{Message: "上传成功", Data: data}, nil
}

// UserInfo 从 token（pd-passport 签发的 JWT）读出用户 ID 并检查是否过期，
// 再查账户信息取昵称（账户接口不返回用户 ID，同时借此确认 token 仍被奈雪接受）。
func (s *NayukiService) UserInfo(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, ErrMissingToken
	}
	claims, err := parseNayukiToken(token)
	if err != nil {
		return domain.User{}, err
	}
	if claims.Exp > 0 {
		if exp := time.Unix(claims.Exp, 0); !s.now().Before(exp) {
			return domain.User{}, fmt.Errorf("%w（到期时间 %s）", ErrTokenExpired, exp.In(cst).Format("2006-01-02 15:04"))
		}
	}
	nickname, err := s.gateway.Nickname(ctx, token)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: claims.UserID, Name: nickname}, nil
}

type nayukiClaims struct {
	UserID string `json:"userId"`
	Exp    int64  `json:"exp"`
}

// parseNayukiToken 只解码 JWT 载荷取字段，不验签（HS256 密钥在奈雪服务端）。
func parseNayukiToken(token string) (nayukiClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nayukiClaims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nayukiClaims{}, ErrInvalidToken
	}
	var c nayukiClaims
	if err := json.Unmarshal(payload, &c); err != nil || c.UserID == "" {
		return nayukiClaims{}, ErrInvalidToken
	}
	return c, nil
}
