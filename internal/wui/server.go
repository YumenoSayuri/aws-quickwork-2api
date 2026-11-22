package wui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"quickwork-openai/internal/accounts"
	"quickwork-openai/internal/config"
	"quickwork-openai/internal/store"
)

// Options configures the console.
type Options struct {
	ConfigPath string
	Binary     string
	WorkDir    string
	Addr       string
	Password   string
	SessionTTL time.Duration
}

// Server is the management console.
type Server struct {
	cfg        *config.Config
	store      *store.Store
	mgr        *accounts.Manager
	opts       Options
	configPath string
	binary     string
	workDir    string
	guard      *guard

	mu    sync.Mutex
	login *Login
}

// New builds the console. The caller keeps ownership of st and must close it.
func New(cfg *config.Config, st *store.Store, opts Options) *Server {
	if opts.ConfigPath == "" {
		opts.ConfigPath = cfg.Path
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Dir(opts.ConfigPath)
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = cfg.WUI.Session.Duration()
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 12 * time.Hour
	}
	if opts.Password == "" {
		opts.Password = cfg.WUI.Password
	}
	abs, err := filepath.Abs(opts.ConfigPath)
	if err == nil {
		opts.ConfigPath = abs
	}
	abs, err = filepath.Abs(opts.Binary)
	if err == nil {
		opts.Binary = abs
	}
	abs, err = filepath.Abs(opts.WorkDir)
	if err == nil {
		opts.WorkDir = abs
	}
	return &Server{
		cfg:        cfg,
		store:      st,
		mgr:        accounts.New(st, cfg),
		opts:       opts,
		configPath: opts.ConfigPath,
		binary:     opts.Binary,
		workDir:    opts.WorkDir,
		guard:      &guard{pw: NewPassword(opts.Password), sess: newSessions(opts.SessionTTL)},
	}
}

// Handler returns the console mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/state", s.guard.require(s.handleState))
	mux.HandleFunc("/api/accounts", s.guard.require(s.handleAccounts))
	mux.HandleFunc("/api/accounts/", s.guard.require(s.handleAccountItem))
	mux.HandleFunc("/api/refresh", s.guard.require(s.handleRefresh))
	mux.HandleFunc("/api/login", s.guard.require(s.handleLoginStart))
	mux.HandleFunc("/api/login/current", s.guard.require(s.handleLoginCurrent))
	mux.HandleFunc("/api/login/callback", s.guard.require(s.handleLoginCallback))
	mux.HandleFunc("/api/login/cancel", s.guard.require(s.handleLoginCancel))
	mux.HandleFunc("/api/config", s.guard.require(s.handleConfig))

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Referrer-Policy", "same-origin")
	_, _ = w.Write([]byte(indexHTML))
}

type sessionReq struct {
	Password string `json:"password"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "authed": s.guard.authorised(r)})
		return
	case http.MethodPost:
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req sessionReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	time.Sleep(250 * time.Millisecond)
	if !s.guard.pw.matches(req.Password) {
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	token, err := s.guard.sess.issue()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "cannot create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.guard.sess.ttl.Seconds()),
	})
	writeOK(w, nil)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.guard.sess.drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeOK(w, nil)
}

type accountView struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Stage       string `json:"build_stage"`
	Region      string `json:"region"`
	WSHost      string `json:"ws_host"`
	AccountName string `json:"account_name"`
	AWSAccount  string `json:"aws_account_id"`
	ExpiresAt   int64  `json:"expires_at"`
	UpdatedAt   int64  `json:"updated_at"`
	LastUsedAt  int64  `json:"last_used_at"`
	UseCount    int64  `json:"use_count"`
	Disabled    bool   `json:"disabled"`
	LastError   string `json:"last_error"`
	HasRefresh  bool   `json:"has_refresh"`
	Status      string `json:"status"`
}

func toView(a *store.Account, now int64) accountView {
	status := "ready"
	switch {
	case a.Disabled:
		status = "disabled"
	case a.ExpiresAt > 0 && a.ExpiresAt <= now:
		status = "expired"
	case a.LastError != "":
		status = "error"
	}
	return accountView{
		ID: a.ID, Email: a.Email, DisplayName: a.DisplayName,
		Provider: a.Provider, Stage: a.BuildStage, Region: a.Region,
		WSHost: a.WSHost, AccountName: a.AccountName, AWSAccount: a.AWSAccountID,
		ExpiresAt: a.ExpiresAt, UpdatedAt: a.UpdatedAt, LastUsedAt: a.LastUsedAt,
		UseCount: a.UseCount, Disabled: a.Disabled, LastError: a.LastError,
		HasRefresh: a.RefreshToken != "", Status: status,
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.List(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取账号失败: "+err.Error())
		return
	}
	now := time.Now().Unix()
	views := make([]accountView, 0, len(list))
	enabled := 0
	for i := range list {
		v := toView(&list[i], now)
		if !list[i].Disabled {
			enabled++
		}
		views = append(views, v)
	}

	s.mu.Lock()
	login := s.login
	s.mu.Unlock()
	var loginView any
	if login != nil {
		loginView = login.snapshot()
	}

	cfgView := map[string]any{
		"addr":            s.cfg.Addr,
		"database":        s.cfg.Database,
		"models":          s.cfg.Models,
		"default_model":   s.cfg.DefaultModel,
		"timezone":        s.cfg.Timezone,
		"locale":          s.cfg.Locale,
		"thinking_effort": s.cfg.ThinkingEffort,
		"api_key_count":   len(s.cfg.APIKeys),
		"api_keys":        s.cfg.APIKeys,
		"provider":        s.cfg.OAuth.Provider,
		"build_stage":     s.cfg.OAuth.BuildStage,
		"callback_port":   s.cfg.OAuth.CallbackPort,
		"oauth_timeout":   s.cfg.OAuth.Timeout.String(),
		"refresh_before":  s.cfg.RefreshBefore.String(),
		"request_timeout": s.cfg.RequestTimeout.String(),
		"app_version":     s.cfg.AppVersion,
		"keep_sessions":   s.cfg.KeepSessions,
		"config_path":     s.cfg.Path,
		"server_addr":     s.opts.Addr,
		"account_total":   len(views),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"now":      now,
		"accounts": views,
		"enabled":  enabled,
		"login":    loginView,
		"config":   cfgView,
	})
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.handleState(w, r)
}

func (s *Server) handleAccountItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, "missing account id")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid account id")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	ctx := r.Context()
	switch r.Method {
	case http.MethodDelete:
		ok, err := s.store.Delete(ctx, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "删除失败: "+err.Error())
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "账号不存在")
			return
		}
		writeOK(w, map[string]any{"id": id, "removed": true})
		return
	case http.MethodPost:
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch action {
	case "disable", "enable":
		if _, err := s.store.Get(ctx, id); err != nil {
			writeErr(w, http.StatusNotFound, "账号不存在")
			return
		}
		if err := s.store.SetDisabled(ctx, id, action == "disable"); err != nil {
			writeErr(w, http.StatusInternalServerError, "更新失败: "+err.Error())
			return
		}
		writeOK(w, map[string]any{"id": id, "disabled": action == "disable"})
	case "refresh":
		a, err := s.mgr.RefreshOne(ctx, id)
		if err != nil {
			_ = s.store.SetError(ctx, id, err.Error())
			writeErr(w, http.StatusBadGateway, "刷新失败: "+err.Error())
			return
		}
		writeOK(w, map[string]any{"account": toView(a, time.Now().Unix())})
	default:
		writeErr(w, http.StatusBadRequest, "unknown action")
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	results, err := s.mgr.RefreshAll(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "刷新失败: "+err.Error())
		return
	}
	out := make([]map[string]any, 0, len(results))
	failed := 0
	for _, res := range results {
		item := map[string]any{}
		if res.Account != nil {
			item["id"] = res.Account.ID
			item["email"] = res.Account.Email
			_ = s.store.SetError(ctx, res.Account.ID, "")
			if res.Err != nil {
				item["error"] = res.Err.Error()
				_ = s.store.SetError(ctx, res.Account.ID, res.Err.Error())
			}
		}
		if res.Err != nil {
			failed++
			if _, has := item["error"]; !has {
				item["error"] = res.Err.Error()
			}
		}
		out = append(out, item)
	}
	writeOK(w, map[string]any{"results": out, "failed": failed, "total": len(out)})
}

type loginStartReq struct {
	Provider string `json:"provider"`
	Stage    string `json:"stage"`
}

func (s *Server) handleLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req loginStartReq
	if r.ContentLength > 0 {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	l, err := s.startLogin(req.Provider, req.Stage, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "启动登录失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"login": l.snapshot()})
}

func (s *Server) currentLogin() *Login {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.login
}

func (s *Server) handleLoginCurrent(w http.ResponseWriter, r *http.Request) {
	l := s.currentLogin()
	if l == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": l.snapshot()})
}

type callbackReq struct {
	URL string `json:"url"`
}

func (s *Server) handleLoginCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	l := s.currentLogin()
	if l == nil {
		writeErr(w, http.StatusBadRequest, "没有进行中的登录，请先点“开始添加账号”")
		return
	}
	var req callbackReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if err := l.submit(req.URL); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	l.settle(6 * time.Second)
	writeOK(w, map[string]any{"login": l.snapshot()})
}

func (s *Server) handleLoginCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if l := s.currentLogin(); l != nil {
		l.stop()
	}
	writeOK(w, nil)
}

type configPatch struct {
	DefaultModel   *string   `json:"default_model"`
	Timezone       *string   `json:"timezone"`
	Locale         *string   `json:"locale"`
	ThinkingEffort *string   `json:"thinking_effort"`
	APIKeys        *[]string `json:"api_keys"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeOK(w, map[string]any{
			"models":          s.cfg.Models,
			"default_model":   s.cfg.DefaultModel,
			"timezone":        s.cfg.Timezone,
			"locale":          s.cfg.Locale,
			"thinking_effort": s.cfg.ThinkingEffort,
			"api_keys":        s.cfg.APIKeys,
			"path":            s.cfg.Path,
		})
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var patch configPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}

	raw, err := os.ReadFile(s.configPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取配置失败: "+err.Error())
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		writeErr(w, http.StatusInternalServerError, "解析配置失败: "+err.Error())
		return
	}
	if patch.DefaultModel != nil {
		v := strings.TrimSpace(*patch.DefaultModel)
		if v != "" {
			doc["default_model"] = v
		}
	}
	if patch.Timezone != nil {
		doc["timezone"] = strings.TrimSpace(*patch.Timezone)
	}
	if patch.Locale != nil {
		doc["locale"] = strings.TrimSpace(*patch.Locale)
	}
	if patch.ThinkingEffort != nil {
		doc["thinking_effort"] = strings.TrimSpace(*patch.ThinkingEffort)
	}
	if patch.APIKeys != nil {
		clean := make([]string, 0, len(*patch.APIKeys))
		for _, k := range *patch.APIKeys {
			k = strings.TrimSpace(k)
			if k != "" {
				clean = append(clean, k)
			}
		}
		if len(clean) == 0 {
			writeErr(w, http.StatusBadRequest, "API key 不能全部清空，否则接口会变成无鉴权")
			return
		}
		doc["api_keys"] = clean
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "序列化配置失败: "+err.Error())
		return
	}
	out = append(out, '\n')
	if err := writeFileAtomic(s.configPath, out); err != nil {
		writeErr(w, http.StatusInternalServerError, "写入配置失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"restart_required": true})
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	_ = os.Chmod(name, 0o600)
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
