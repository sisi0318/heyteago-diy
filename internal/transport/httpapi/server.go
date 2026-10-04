// Package httpapi 对外暴露 JSON API。只用 net/http 标准库（Go 1.22 方法路由）。
// 路由按平台分组为 /api/{platform}/...；token 不落日志；multipart 请求体按领域上限截断。
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DiheMoe/heyteago-diy/internal/domain"
	"github.com/DiheMoe/heyteago-diy/internal/usecase"
)

// 各平台用例对 transport 暴露的能力。
type (
	StickerUploader interface {
		Upload(ctx context.Context, in usecase.StickerUpload) (usecase.UploadOutput, error)
	}
	DraftSaver interface {
		SaveDraft(ctx context.Context, in usecase.DraftSave) (usecase.UploadOutput, error)
	}
	UserLookup interface {
		UserInfo(ctx context.Context, token string) (domain.User, error)
	}
	PhoneAuth interface {
		SendLoginSms(ctx context.Context, phone, ticket, randstr string) error
		Login(ctx context.Context, phone, code, ticket string) (usecase.AuthOutput, error)
	}
)

// Platform 是一个奶茶平台的能力集合：Stickers、Users 必填；
// Drafts、Auth 为 nil 表示该平台不支持，对应路由返回 404。
type Platform struct {
	Stickers StickerUploader
	Users    UserLookup
	Drafts   DraftSaver
	Auth     PhoneAuth
}

type Server struct {
	platforms map[string]Platform
}

// NewServer 的 platforms 以路由中的平台标识为键（如 heytea、nayuki）。
func NewServer(platforms map[string]Platform) *Server {
	return &Server{platforms: platforms}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/{platform}/upload", s.handleUpload)
	mux.HandleFunc("POST /api/{platform}/draft/save", s.handleSaveDraft)
	mux.HandleFunc("GET /api/{platform}/user", s.handleUser)
	mux.HandleFunc("POST /api/{platform}/login/sms", s.handleLoginSms)
	mux.HandleFunc("POST /api/{platform}/login", s.handleLogin)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	return logRequests(mux)
}

// platform 取路径中 {platform} 对应的平台；未知平台直接写 404。
func (s *Server) platform(w http.ResponseWriter, r *http.Request) (Platform, bool) {
	p, ok := s.platforms[r.PathValue("platform")]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "未知平台"})
	}
	return p, ok
}

func writeUnsupported(w http.ResponseWriter, feature string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"message": "该平台不支持" + feature})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	p, ok := s.platform(w, r)
	if !ok {
		return
	}
	file, header, err := parseFilePart(r)
	if err != nil {
		writeError(w, err)
		return
	}

	form := r.MultipartForm.Value
	out, err := p.Stickers.Upload(r.Context(), usecase.StickerUpload{
		Token:       firstValue(form, "token"),
		UserID:      firstValue(form, "userId"),
		FileName:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		File:        file,
		// 缺省为 0，由需要画布尺寸的平台用例补默认值
		Width:  parseIntOr(firstValue(form, "width"), 0),
		Height: parseIntOr(firstValue(form, "height"), 0),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	p, ok := s.platform(w, r)
	if !ok {
		return
	}
	if p.Drafts == nil {
		writeUnsupported(w, "草稿")
		return
	}
	file, header, err := parseFilePart(r)
	if err != nil {
		writeError(w, err)
		return
	}

	out, err := p.Drafts.SaveDraft(r.Context(), usecase.DraftSave{
		Token:       firstValue(r.MultipartForm.Value, "token"),
		FileName:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		File:        file,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	p, ok := s.platform(w, r)
	if !ok {
		return
	}
	provided := r.URL.Query().Get("token")
	if auth := r.Header.Get("Authorization"); auth != "" {
		provided = strings.TrimPrefix(auth, "Bearer ")
	}
	user, err := p.Users.UserInfo(r.Context(), provided)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLoginSms(w http.ResponseWriter, r *http.Request) {
	p, ok := s.platform(w, r)
	if !ok {
		return
	}
	if p.Auth == nil {
		writeUnsupported(w, "短信登录")
		return
	}
	var in struct {
		Phone   string `json:"phone"`
		Ticket  string `json:"ticket"`
		Randstr string `json:"randstr"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := p.Auth.SendLoginSms(r.Context(), in.Phone, in.Ticket, in.Randstr); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.platform(w, r)
	if !ok {
		return
	}
	if p.Auth == nil {
		writeUnsupported(w, "短信登录")
		return
	}
	var in struct {
		Phone  string `json:"phone"`
		Code   string `json:"code"`
		Ticket string `json:"ticket"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	out, err := p.Auth.Login(r.Context(), in.Phone, in.Code, in.Ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// errBadJSON 表示请求体不是合法 JSON，映射为 400。
var errBadJSON = errors.New("请求体不是合法的 JSON")

// decodeJSON 解析 JSON 请求体；登录类端点字段少，请求体限 4KB。
func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errBadJSON
	}
	return nil
}

// parseFilePart 解析 multipart 表单并取出 file 字段的全部字节。
// 请求体上限 = 领域上限 + 1MiB 表单开销，超限直接拒绝。
func parseFilePart(r *http.Request) ([]byte, *multipart.FileHeader, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, domain.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(domain.MaxUploadBytes); err != nil {
		return nil, nil, errors.New("请求体解析失败（可能超过大小上限）")
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, usecase.ErrMissingFile
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || len(data) == 0 {
		return nil, nil, usecase.ErrMissingFile
	}
	return data, header, nil
}

func firstValue(values map[string][]string, key string) string {
	if v := values[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func parseIntOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	body := map[string]any{"message": err.Error()}

	var be *usecase.BusinessError
	var se *usecase.SignError
	var ue *usecase.UpstreamError
	switch {
	case errors.Is(err, usecase.ErrMissingToken),
		errors.Is(err, usecase.ErrInvalidToken),
		errors.Is(err, usecase.ErrTokenExpired),
		errors.Is(err, usecase.ErrMissingUserID),
		errors.Is(err, usecase.ErrMissingFile),
		errors.Is(err, usecase.ErrFileTooLarge),
		errors.Is(err, usecase.ErrInvalidPhone),
		errors.Is(err, usecase.ErrMissingSmsCode),
		errors.Is(err, usecase.ErrMissingTicket),
		errors.Is(err, errBadJSON):
		status = http.StatusBadRequest
	case errors.As(err, &be):
		status = http.StatusBadRequest
		body["message"] = be.Message
		body["code"] = be.Code
	case errors.As(err, &se), errors.As(err, &ue):
		status = http.StatusBadGateway
	default:
		if strings.Contains(err.Error(), "喜茶") {
			status = http.StatusBadGateway
		}
	}
	writeJSON(w, status, body)
}

// logRequests 记录方法与路径，绝不记录 body 与 token。
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[http] %s %s -> %d (%s)", r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
