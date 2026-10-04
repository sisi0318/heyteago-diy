// Package nayukiapi 是奈雪点单小程序通道（tm-api.pin-dao.cn）的 HTTP 客户端。
// 请求形状与小程序抓包一致：JSON 体 {common, params}，common 带 HMAC-SHA1 签名；
// 杯贴图片先取 STS 凭证直传阿里云 OSS，再把图片 URL 提交为作品。
// 抓包里的 lat2/lng2/iv 是加密定位头，接口不依赖，不发送。
package nayukiapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DiheMoe/heyteago-diy/internal/domain"
	"github.com/DiheMoe/heyteago-diy/internal/usecase"
)

const (
	defaultBaseURL = "https://tm-api.pin-dao.cn"
	accountPath    = "/home/api/my/account"
	assumeRolePath = "/comment/getOssAssumeRole"
	saveWorkPath   = "/activity/cupSticker/work/save"

	// signSecret 与 openID 是小程序内置常量：
	// signature = base64(HMAC-SHA1(signSecret, "nonce=<n>&openId=<openID>&timestamp=<秒>"))。
	signSecret = "sArMTldQ9tqU19XIRDMWz7BO5WaeBnrezA"
	openID     = "QL6ZOftGzbziPlZwfiXM"

	// 以下为抓包原值，固定不变：小程序 appId/版本、设备、门店与杯贴活动。
	appID      = "wxab7430e6e8b9a4ab"
	appVersion = "6.0.84"
	deviceOSN  = "2510DRK44C"
	deviceSV   = "Android 16"
	storeID    = 26076222
	activityID = "2187"
	userAgent  = "Mozilla/5.0 (Linux; Android 16; 2510DRK44C Build/BP2A.250605.031.A3; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/150.0.7871.189 Mobile Safari/537.36 XWEB/1500145 MMWEBSDK/20260502 MMWEBID/5887 MicroMessenger/8.0.76.3141(0x28004C54) WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64 MiniProgramEnv/android"
	referer    = "https://servicewechat.com/wxab7430e6e8b9a4ab/869/page-frame.html"
)

type Client struct {
	http    *http.Client
	baseURL string           // 可被测试覆盖
	now     func() time.Time // 测试注入固定时钟
	nonce   func() int       // 测试注入固定随机数
}

func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: defaultBaseURL,
		now:     time.Now,
		nonce:   func() int { return rand.IntN(1_000_000) },
	}
}

// common 是每个请求体都带的客户端信息，字段顺序与抓包一致；
// nonce/timestamp/signature 每次请求现算。
type common struct {
	Platform  string `json:"platform"`
	Version   string `json:"version"`
	IMEI      string `json:"imei"`
	OSN       string `json:"osn"`
	SV        string `json:"sv"`
	Lat       string `json:"lat"`
	Lng       string `json:"lng"`
	Lang      string `json:"lang"`
	Currency  string `json:"currency"`
	TimeZone  string `json:"timeZone"`
	Nonce     int    `json:"nonce"`
	OpenID    string `json:"openId"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

// storeParams 是两个接口共有的门店/渠道参数（抓包原值）。
// storeId 在取凭证接口里是数字、在保存作品接口里是字符串，照抓包原样。
type storeParams struct {
	BusinessType int    `json:"businessType"`
	Brand        int    `json:"brand"`
	TenantID     int    `json:"tenantId"`
	Channel      int    `json:"channel"`
	StallType    string `json:"stallType"`
	StoreID      any    `json:"storeId"`
	StoreType    int    `json:"storeType"`
	CityID       int    `json:"cityId"`
	DistrictID   int    `json:"districtId"`
	AppID        string `json:"appId"`
	DAID         int    `json:"dAId"`
}

type saveWorkParams struct {
	storeParams
	ImageURL   string `json:"imageUrl"`
	WorkStatus int    `json:"workStatus"`
	ActivityID string `json:"activityId"`
}

func newStoreParams(storeID any) storeParams {
	return storeParams{
		BusinessType: 1,
		Brand:        26000252,
		TenantID:     1,
		Channel:      3,
		StallType:    "PD_S_004",
		StoreID:      storeID,
		StoreType:    2,
		CityID:       440800,
		DistrictID:   440803,
		AppID:        appID,
		DAID:         100003,
	}
}

// UploadImage 取 OSS 临时凭证，按小程序的对象路径 comment/<毫秒时间戳>.png 直传，返回图片 URL。
func (c *Client) UploadImage(ctx context.Context, token, contentType string, file []byte) (string, error) {
	creds, err := c.assumeRole(ctx, token)
	if err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = "image/png"
	}
	ext := ".png"
	if contentType == "image/jpeg" {
		ext = ".jpg"
	}
	key := "comment/" + strconv.FormatInt(c.now().UnixMilli(), 10) + ext
	if err := c.postObject(ctx, creds, key, contentType, file); err != nil {
		return "", err
	}
	return strings.TrimRight(creds.BucketURL, "/") + "/" + key, nil
}

// SaveWork 把 OSS 上的图片提交为当前杯贴活动的作品，返回上游 data（含 workId 与审核状态）。
func (c *Client) SaveWork(ctx context.Context, token, imageURL string) (json.RawMessage, error) {
	return c.postJSON(ctx, saveWorkPath, token, saveWorkParams{
		storeParams: newStoreParams(strconv.Itoa(storeID)),
		ImageURL:    imageURL,
		WorkStatus:  2,
		ActivityID:  activityID,
	})
}

// Nickname 查“我的”页账户信息，取昵称（未设置昵称时上游返回“亲爱的用户”）。
func (c *Client) Nickname(ctx context.Context, token string) (string, error) {
	data, err := c.postJSON(ctx, accountPath, token, newStoreParams(storeID))
	if err != nil {
		return "", err
	}
	var account struct {
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(data, &account); err != nil {
		return "", upstream("奈雪账户信息解析失败: %.200s", data)
	}
	return account.Nickname, nil
}

func (c *Client) assumeRole(ctx context.Context, token string) (ossCredentials, error) {
	data, err := c.postJSON(ctx, assumeRolePath, token, newStoreParams(storeID))
	if err != nil {
		return ossCredentials{}, err
	}
	var creds ossCredentials
	if err := json.Unmarshal(data, &creds); err != nil ||
		creds.AccessKeyID == "" || creds.AccessKeySecret == "" || creds.BucketURL == "" {
		// 不回显 data：里面是临时凭证
		return ossCredentials{}, upstream("奈雪 OSS 凭证不完整")
	}
	return creds, nil
}

// postJSON 发送 {common, params} 请求体并按 {code,message,data} 信封解析；
// code!=0 返回 BusinessError，网络/HTTP/解析失败返回 UpstreamError。
func (c *Client) postJSON(ctx context.Context, path, token string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		Common common `json:"common"`
		Params any    `json:"params"`
	}{c.common(), params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setHeaders(req, token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, upstream("请求奈雪失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, upstream("读取奈雪响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, upstream("奈雪返回 HTTP %d: %.200s", resp.StatusCode, raw)
	}
	var res domain.Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, upstream("奈雪响应解析失败: %.200s", raw)
	}
	if res.Code != 0 {
		return nil, &usecase.BusinessError{Code: res.Code, Message: res.Message}
	}
	return res.Data, nil
}

func (c *Client) common() common {
	nonce, ts := c.nonce(), c.now().Unix()
	return common{
		Platform:  "wxapp",
		Version:   appVersion,
		OSN:       deviceOSN,
		SV:        deviceSV,
		Lang:      "zh_CN",
		Currency:  "CNY",
		Nonce:     nonce,
		OpenID:    openID,
		Timestamp: ts,
		Signature: sign(nonce, ts),
	}
}

// sign 计算请求体 common.signature。
func sign(nonce int, timestamp int64) string {
	mac := hmac.New(sha1.New, []byte(signSecret))
	fmt.Fprintf(mac, "nonce=%d&openId=%s&timestamp=%d", nonce, openID, timestamp)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// setHeaders 与小程序抓包一致（去掉加密定位头）；
// 不设 Accept-Encoding，交给 net/http 自动协商并解压 gzip。
func setHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("charset", "utf-8")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("storeid", strconv.Itoa(storeID))
	req.Header.Set("Referer", referer)
}

func upstream(format string, args ...any) error {
	return &usecase.UpstreamError{Err: fmt.Errorf(format, args...)}
}
