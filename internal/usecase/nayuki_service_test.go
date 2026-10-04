package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DiheMoe/heyteago-diy/internal/domain"
)

type fakeNayukiGateway struct {
	calls       []string
	imageURL    string
	uploadErr   error
	saveErr     error
	nickname    string
	nicknameErr error
	savedURL    string
	contentType string
}

func (f *fakeNayukiGateway) UploadImage(_ context.Context, token, contentType string, _ []byte) (string, error) {
	f.calls = append(f.calls, "upload:"+token)
	f.contentType = contentType
	return f.imageURL, f.uploadErr
}

func (f *fakeNayukiGateway) SaveWork(_ context.Context, token, imageURL string) (json.RawMessage, error) {
	f.calls = append(f.calls, "save:"+token)
	f.savedURL = imageURL
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return json.RawMessage(`{"workId":122374}`), nil
}

func (f *fakeNayukiGateway) Nickname(_ context.Context, token string) (string, error) {
	f.calls = append(f.calls, "nickname:"+token)
	return f.nickname, f.nicknameErr
}

// fakeJWT 拼一个只有载荷有意义的 JWT（签名段随意，用例不验签）。
func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "eyJhbGciOiJIUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestNayukiUploadFlow(t *testing.T) {
	gw := &fakeNayukiGateway{imageURL: "https://oss.example/comment/1.png"}
	svc := NewNayukiService(gw)

	out, err := svc.Upload(context.Background(), StickerUpload{Token: "jwt", ContentType: "image/png", File: []byte("png")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gw.calls) != 2 || gw.calls[0] != "upload:jwt" || gw.calls[1] != "save:jwt" {
		t.Fatalf("calls = %v, want [upload save]", gw.calls)
	}
	if gw.savedURL != gw.imageURL || gw.contentType != "image/png" {
		t.Fatalf("savedURL=%q contentType=%q", gw.savedURL, gw.contentType)
	}
	if out.Message != "上传成功" || string(out.Data) != `{"workId":122374}` {
		t.Fatalf("out = %+v", out)
	}
}

func TestNayukiUploadStopsWhenImageFails(t *testing.T) {
	gw := &fakeNayukiGateway{uploadErr: &UpstreamError{Err: errors.New("oss down")}}
	svc := NewNayukiService(gw)

	_, err := svc.Upload(context.Background(), StickerUpload{Token: "jwt", File: []byte("png")})
	var ue *UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UpstreamError", err)
	}
	if len(gw.calls) != 1 {
		t.Fatalf("calls = %v, SaveWork must not run after upload failure", gw.calls)
	}
}

func TestNayukiUploadValidation(t *testing.T) {
	svc := NewNayukiService(&fakeNayukiGateway{})
	cases := []struct {
		name string
		in   StickerUpload
		want error
	}{
		{"missing token", StickerUpload{File: []byte("x")}, ErrMissingToken},
		{"missing file", StickerUpload{Token: "jwt"}, ErrMissingFile},
		{"file too large", StickerUpload{Token: "jwt", File: make([]byte, domain.MaxUploadBytes+1)}, ErrFileTooLarge},
	}
	for _, c := range cases {
		if _, err := svc.Upload(context.Background(), c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestNayukiUserInfo(t *testing.T) {
	now := time.Unix(1791089341, 0)
	token := fakeJWT(t, map[string]any{"userId": "224307153", "sub": "224307153", "exp": now.Unix() + 3600})
	gw := &fakeNayukiGateway{nickname: "亲爱的用户"}
	svc := NewNayukiService(gw)
	svc.now = func() time.Time { return now }

	user, err := svc.UserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != "224307153" || user.Name != "亲爱的用户" {
		t.Fatalf("user = %+v", user)
	}
	if len(gw.calls) != 1 || gw.calls[0] != "nickname:"+token {
		t.Fatalf("calls = %v, want one Nickname", gw.calls)
	}
}

func TestNayukiUserInfoExpired(t *testing.T) {
	now := time.Unix(1791089341, 0)
	token := fakeJWT(t, map[string]any{"userId": "1", "exp": now.Unix()})
	gw := &fakeNayukiGateway{}
	svc := NewNayukiService(gw)
	svc.now = func() time.Time { return now }

	if _, err := svc.UserInfo(context.Background(), token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
	if len(gw.calls) != 0 {
		t.Fatalf("calls = %v, expired token must not hit upstream", gw.calls)
	}
}

func TestNayukiUserInfoInvalidToken(t *testing.T) {
	svc := NewNayukiService(&fakeNayukiGateway{})
	for _, token := range []string{
		"not-a-jwt",
		"a.b",
		"a.!!!.c",
		fakeJWT(t, map[string]any{"sub": "1"}), // 缺 userId
	} {
		if _, err := svc.UserInfo(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("token %q: err = %v, want ErrInvalidToken", token, err)
		}
	}
	if _, err := svc.UserInfo(context.Background(), ""); !errors.Is(err, ErrMissingToken) {
		t.Errorf("empty token: err = %v, want ErrMissingToken", err)
	}
}

func TestNayukiUserInfoRejectedUpstream(t *testing.T) {
	gw := &fakeNayukiGateway{nicknameErr: &BusinessError{Code: 401, Message: "登录已失效"}}
	svc := NewNayukiService(gw)
	svc.now = func() time.Time { return time.Unix(0, 0) }

	_, err := svc.UserInfo(context.Background(), fakeJWT(t, map[string]any{"userId": "1", "exp": 100}))
	var be *BusinessError
	if !errors.As(err, &be) || be.Code != 401 {
		t.Fatalf("err = %v, want BusinessError 401", err)
	}
}
