package nayukiapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"time"
)

// ossMaxBytes 是小程序 policy 里 content-length-range 的上限（1GiB）。
const ossMaxBytes = 1 << 30

// ossCredentials 是取凭证接口下发的 STS 临时凭证与 bucket 地址。
type ossCredentials struct {
	AccessKeyID     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	SecurityToken   string `json:"securityToken"`
	BucketURL       string `json:"bucketUrl"`
}

// postObject 按阿里云 OSS PostObject 表单直传：字段顺序与小程序一致，file 必须放最后。
func (c *Client) postObject(ctx context.Context, creds ossCredentials, key, contentType string, file []byte) error {
	policy := ossPolicy(c.now())
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, f := range [][2]string{
		{"OSSAccessKeyId", creds.AccessKeyID},
		{"signature", ossSignature(creds.AccessKeySecret, policy)},
		{"x-oss-security-token", creds.SecurityToken},
		{"key", key},
		{"policy", policy},
	} {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return err
		}
	}
	filename, err := tempFileName(path.Ext(key))
	if err != nil {
		return err
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	header.Set("Content-Type", contentType)
	part, err := w.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := part.Write(file); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, creds.BucketURL, &body)
	if err != nil {
		return upstream("奈雪 OSS 地址无效: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", referer)

	resp, err := c.http.Do(req)
	if err != nil {
		return upstream("上传图片到奈雪 OSS 失败: %w", err)
	}
	defer resp.Body.Close()
	// 成功为 204 无响应体；失败时 OSS 返回 XML（含 Code/Message）
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return upstream("奈雪 OSS 返回 HTTP %d: %.300s", resp.StatusCode, raw)
	}
	return nil
}

// ossPolicy 生成 PostObject 的 base64 policy：1 小时后过期，只限制文件大小（与小程序一致）。
func ossPolicy(now time.Time) string {
	raw, _ := json.Marshal(struct {
		Expiration string  `json:"expiration"`
		Conditions [][]any `json:"conditions"`
	}{
		Expiration: now.Add(time.Hour).UTC().Format("2006-01-02T15:04:05.000Z"),
		Conditions: [][]any{{"content-length-range", 0, ossMaxBytes}},
	})
	return base64.StdEncoding.EncodeToString(raw)
}

// ossSignature 是 OSS PostObject V1 签名：base64(HMAC-SHA1(AccessKeySecret, policy))。
func ossSignature(secret, policy string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(policy))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// tempFileName 仿微信临时文件名 tmp_<48 位十六进制><ext>；OSS 按 key 存储，文件名只影响表单形状。
func tempFileName(ext string) (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "tmp_" + hex.EncodeToString(b[:]) + ext, nil
}
