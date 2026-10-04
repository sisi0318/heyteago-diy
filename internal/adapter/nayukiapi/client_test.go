package nayukiapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DiheMoe/heyteago-diy/internal/usecase"
)

// 已知答案向量取自奈雪小程序抓包（2026-10-04，小程序 6.0.84）。

func TestSignKnownAnswers(t *testing.T) {
	cases := []struct {
		nonce int
		ts    int64
		want  string
	}{
		{574048, 1791089341, "DnpsWgBjgcHbOVYW1MTgGL2+ndg="}, // getOssAssumeRole
		{70555, 1791089342, "2SYUpHMsVPyumr7zStp/yWN2Xzc="},  // cupSticker/work/save
		{130440, 1791092009, "aXPfDTmNfsF8F+uMr7DwJBxQcyk="}, // home/api/my/account
	}
	for _, c := range cases {
		if got := sign(c.nonce, c.ts); got != c.want {
			t.Errorf("sign(%d, %d) = %s, want %s", c.nonce, c.ts, got, c.want)
		}
	}
}

func TestOSSPolicyKnownAnswer(t *testing.T) {
	const (
		wantPolicy = "eyJleHBpcmF0aW9uIjoiMjAyNi0xMC0wNFQwNTo0OTowMi4wOTJaIiwiY29uZGl0aW9ucyI6W1siY29udGVudC1sZW5ndGgtcmFuZ2UiLDAsMTA3Mzc0MTgyNF1dfQ=="
		// 抓包当次的 STS 临时密钥（已过期），仅用于复核签名算法
		secret  = "CerNagWGWqgdBRcLcDUr2e2SHaGbGBduZSYHEKA9ZMNN"
		wantSig = "q7TTJMG0bT6tCiZ2Rr3vbwuVJto="
	)
	policy := ossPolicy(time.UnixMilli(1791089342092))
	if policy != wantPolicy {
		raw, _ := base64.StdEncoding.DecodeString(policy)
		t.Fatalf("policy = %s", raw)
	}
	if got := ossSignature(secret, policy); got != wantSig {
		t.Errorf("ossSignature = %s, want %s", got, wantSig)
	}
}

// newTestClient 的时钟与 nonce 固定为抓包第一条请求的取值，签名可与抓包逐字比对。
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New()
	c.baseURL = srv.URL
	c.now = func() time.Time { return time.UnixMilli(1791089341500) }
	c.nonce = func() int { return 574048 }
	return c, srv
}

func writeCredentials(w http.ResponseWriter, bucketURL string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    0,
		"message": "",
		"data": map[string]any{
			"securityToken":   "sec-token",
			"accessKeySecret": "test-secret",
			"accessKeyId":     "STS.test",
			"bucketUrl":       bucketURL,
			"expirationTime":  "2026-10-04T05:32:36Z",
		},
		"dataType": "CredentialsVO",
		"success":  true,
	})
}

type ossForm struct {
	order    []string
	fields   map[string]string
	filename string
	ctype    string
	file     string
}

func readOSSForm(t *testing.T, r *http.Request) ossForm {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("bad Content-Type: %v", err)
	}
	form := ossForm{fields: map[string]string{}}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		data, _ := io.ReadAll(part)
		form.order = append(form.order, part.FormName())
		if part.FormName() == "file" {
			form.filename = part.FileName()
			form.ctype = part.Header.Get("Content-Type")
			form.file = string(data)
		} else {
			form.fields[part.FormName()] = string(data)
		}
	}
	return form
}

// 断言整条上传链路与小程序抓包一致：取凭证的头与签名体、OSS 表单字段顺序与签名、返回 URL。
func TestUploadImageFlow(t *testing.T) {
	var (
		stsHeaders http.Header
		stsBody    map[string]map[string]any
		ossHeaders http.Header
		form       ossForm
	)
	var bucketURL string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case assumeRolePath:
			stsHeaders = r.Header.Clone()
			_ = json.NewDecoder(r.Body).Decode(&stsBody)
			writeCredentials(w, bucketURL)
		case "/bucket":
			ossHeaders = r.Header.Clone()
			form = readOSSForm(t, r)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	bucketURL = srv.URL + "/bucket"

	url, err := c.UploadImage(context.Background(), "tok", "image/png", []byte("PNGDATA"))
	if err != nil {
		t.Fatalf("UploadImage error: %v", err)
	}
	if want := bucketURL + "/comment/1791089341500.png"; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}

	// 取凭证：头与抓包一致（不含加密定位头）
	wantHeaders := map[string]string{
		"Authorization": "Bearer tok",
		"Storeid":       "26076222",
		"Charset":       "utf-8",
		"Content-Type":  "application/json",
		"Referer":       referer,
		"User-Agent":    userAgent,
	}
	for k, v := range wantHeaders {
		if got := stsHeaders.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	for _, h := range []string{"lat2", "lng2", "iv"} {
		if stsHeaders.Get(h) != "" {
			t.Errorf("unexpected header %s", h)
		}
	}
	common, params := stsBody["common"], stsBody["params"]
	if common["signature"] != "DnpsWgBjgcHbOVYW1MTgGL2+ndg=" || common["nonce"] != 574048.0 ||
		common["timestamp"] != 1791089341.0 || common["openId"] != openID || common["platform"] != "wxapp" {
		t.Errorf("common = %v", common)
	}
	if params["storeId"] != 26076222.0 || params["appId"] != appID || params["stallType"] != "PD_S_004" {
		t.Errorf("params = %v (storeId 应为数字)", params)
	}

	// OSS 直传：字段顺序、policy 与签名、文件 part
	wantOrder := []string{"OSSAccessKeyId", "signature", "x-oss-security-token", "key", "policy", "file"}
	if strings.Join(form.order, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("form order = %v, want %v", form.order, wantOrder)
	}
	if form.fields["OSSAccessKeyId"] != "STS.test" || form.fields["x-oss-security-token"] != "sec-token" ||
		form.fields["key"] != "comment/1791089341500.png" {
		t.Errorf("form fields = %v", form.fields)
	}
	policy, _ := base64.StdEncoding.DecodeString(form.fields["policy"])
	if string(policy) != `{"expiration":"2026-10-04T05:49:01.500Z","conditions":[["content-length-range",0,1073741824]]}` {
		t.Errorf("policy = %s", policy)
	}
	if form.fields["signature"] != ossSignature("test-secret", form.fields["policy"]) {
		t.Errorf("signature = %q", form.fields["signature"])
	}
	if !regexp.MustCompile(`^tmp_[0-9a-f]{48}\.png$`).MatchString(form.filename) {
		t.Errorf("filename = %q", form.filename)
	}
	if form.ctype != "image/png" || form.file != "PNGDATA" {
		t.Errorf("file part = %q (%q)", form.file, form.ctype)
	}
	if ossHeaders.Get("Referer") != referer || ossHeaders.Get("User-Agent") != userAgent {
		t.Errorf("oss headers = %v", ossHeaders)
	}
}

func TestUploadImageJPEGKey(t *testing.T) {
	var form ossForm
	var bucketURL string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == assumeRolePath {
			writeCredentials(w, bucketURL)
			return
		}
		form = readOSSForm(t, r)
		w.WriteHeader(http.StatusNoContent)
	})
	bucketURL = srv.URL + "/bucket/" // 末尾斜杠不产生双斜杠

	url, err := c.UploadImage(context.Background(), "tok", "image/jpeg", []byte("JPG"))
	if err != nil {
		t.Fatalf("UploadImage error: %v", err)
	}
	if want := srv.URL + "/bucket/comment/1791089341500.jpg"; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if form.fields["key"] != "comment/1791089341500.jpg" || form.ctype != "image/jpeg" {
		t.Errorf("key=%q ctype=%q", form.fields["key"], form.ctype)
	}
}

func TestSaveWorkRequestShape(t *testing.T) {
	var gotPath string
	var body map[string]map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"workId":122374,"auditStatus":1},"dataType":"CustomizedCupStickerWorkDTO","success":true}`))
	})

	data, err := c.SaveWork(context.Background(), "tok", "https://oss.example/comment/1.png")
	if err != nil {
		t.Fatalf("SaveWork error: %v", err)
	}
	if gotPath != saveWorkPath {
		t.Errorf("path = %q", gotPath)
	}
	params := body["params"]
	if params["storeId"] != "26076222" || params["imageUrl"] != "https://oss.example/comment/1.png" ||
		params["workStatus"] != 2.0 || params["activityId"] != activityID || params["dAId"] != 100003.0 {
		t.Errorf("params = %v (storeId 应为字符串)", params)
	}
	if body["common"]["signature"] != "DnpsWgBjgcHbOVYW1MTgGL2+ndg=" {
		t.Errorf("common = %v", body["common"])
	}
	if string(data) != `{"workId":122374,"auditStatus":1}` {
		t.Errorf("data = %s", data)
	}
}

func TestNicknameRequestShape(t *testing.T) {
	var gotPath, gotAuth string
	var body map[string]map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"nickname":"亲爱的用户","level":1},"dataType":"UserAccountInfoVO","success":true}`))
	})

	nickname, err := c.Nickname(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Nickname error: %v", err)
	}
	if nickname != "亲爱的用户" {
		t.Errorf("nickname = %q", nickname)
	}
	if gotPath != accountPath || gotAuth != "Bearer tok" {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
	if body["params"]["storeId"] != 26076222.0 || body["common"]["signature"] != "DnpsWgBjgcHbOVYW1MTgGL2+ndg=" {
		t.Errorf("body = %v", body)
	}
}

func TestBusinessCodeSurfaces(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":401,"message":"请重新登录","data":null,"success":false}`))
	})
	_, err := c.Nickname(context.Background(), "tok")
	var be *usecase.BusinessError
	if !errors.As(err, &be) || be.Code != 401 || be.Message != "请重新登录" {
		t.Fatalf("err = %v, want BusinessError 401", err)
	}
}

func TestUpstreamErrors(t *testing.T) {
	t.Run("HTTP 非 200", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
		})
		_, err := c.Nickname(context.Background(), "tok")
		var ue *usecase.UpstreamError
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), "502") {
			t.Fatalf("err = %v, want UpstreamError with 502", err)
		}
	})

	t.Run("凭证不完整", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"code":0,"message":"","data":{}}`))
		})
		_, err := c.UploadImage(context.Background(), "tok", "image/png", []byte("x"))
		var ue *usecase.UpstreamError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %v, want UpstreamError", err)
		}
	})

	t.Run("OSS 拒绝", func(t *testing.T) {
		var bucketURL string
		c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == assumeRolePath {
				writeCredentials(w, bucketURL)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
		})
		bucketURL = srv.URL + "/bucket"
		_, err := c.UploadImage(context.Background(), "tok", "image/png", []byte("x"))
		var ue *usecase.UpstreamError
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "AccessDenied") {
			t.Fatalf("err = %v, want UpstreamError with 403 AccessDenied", err)
		}
	})
}
