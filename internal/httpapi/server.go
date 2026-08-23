// Package httpapi 提供 REST 路由层，统一以 /api 前缀暴露能力。
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"task191-reproof/internal/model"
	"task191-reproof/internal/service"
)

// Server 是 HTTP 处理器。
type Server struct {
	app *service.App
	mux *http.ServeMux
}

// New 构造 HTTP 服务器并注册全部路由。
func New(app *service.App) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回根处理器（供 net/http 使用）。
func (s *Server) Handler() http.Handler { return s.mux }

// routes 注册全部路由。
func (s *Server) routes() {
	// 系统
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /", s.handleIndex)

	// 目标
	s.mux.HandleFunc("POST /api/targets", s.handleCreateTarget)
	s.mux.HandleFunc("GET /api/targets", s.handleListTargets)
	s.mux.HandleFunc("GET /api/targets/{id}", s.handleGetTarget)
	s.mux.HandleFunc("POST /api/targets/{id}/seeds", s.handleAddSeed)
	s.mux.HandleFunc("GET /api/targets/{id}/seeds", s.handleListSeeds)

	// 动作（目标下创建、列出）
	s.mux.HandleFunc("POST /api/targets/{id}/actions", s.handleCreateAction)
	s.mux.HandleFunc("GET /api/targets/{id}/actions", s.handleListActions)
	s.mux.HandleFunc("GET /api/targets/{id}/declarations", s.handleListDeclarations)

	// 动作详情
	s.mux.HandleFunc("GET /api/actions/{id}", s.handleGetAction)
	s.mux.HandleFunc("POST /api/actions/{id}/deps", s.handleAddDep)
	s.mux.HandleFunc("POST /api/actions/{id}/declarations", s.handleAddDeclaration)
	s.mux.HandleFunc("POST /api/actions/{id}/toolchains", s.handleAddToolchain)
	s.mux.HandleFunc("POST /api/actions/{id}/logs", s.handleAppendLogs)
	s.mux.HandleFunc("GET /api/actions/{id}/logs", s.handleListActionLogs)

	// 分析
	s.mux.HandleFunc("POST /api/targets/{id}/analyze", s.handleAnalyze)
	s.mux.HandleFunc("GET /api/targets/{id}/violations", s.handleListViolations)
	s.mux.HandleFunc("GET /api/targets/{id}/chains", s.handleListChains)
	s.mux.HandleFunc("GET /api/targets/{id}/pollution", s.handleShortestPollution)

	// 证明与基线
	s.mux.HandleFunc("POST /api/targets/{id}/proofs", s.handleGenerateProof)
	s.mux.HandleFunc("GET /api/targets/{id}/proofs", s.handleListProofs)
	s.mux.HandleFunc("GET /api/proofs/{id}", s.handleGetProof)
	s.mux.HandleFunc("POST /api/proofs/{id}/invalidate", s.handleInvalidateProof)
	s.mux.HandleFunc("POST /api/targets/{id}/baseline", s.handleFreezeBaseline)
	s.mux.HandleFunc("GET /api/targets/{id}/baselines", s.handleListBaselines)
	s.mux.HandleFunc("POST /api/targets/{id}/compare", s.handleCompareBaseline)
}

// --- 请求/响应辅助 ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, model.ErrDuplicateName),
		errors.Is(err, model.ErrCycleDetected),
		errors.Is(err, model.ErrBaselineFrozen),
		errors.Is(err, model.ErrTargetNotProven):
		code = http.StatusConflict
	case errors.Is(err, model.ErrInvalidTransition),
		errors.Is(err, model.ErrEmptyPath),
		errors.Is(err, model.ErrInvalidDirection),
		errors.Is(err, model.ErrDeclarationMissing),
		errors.Is(err, model.ErrConflictLog):
		code = http.StatusInternalServerError
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

type reqBody struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Command     string          `json:"command"`
	DependsOn   int64           `json:"depends_on"`
	Path        string          `json:"path"`
	Direction   string          `json:"direction"`
	Kind        string          `json:"kind"`
	Version     string          `json:"version"`
	Checksum    string          `json:"checksum"`
	Logs        []logEntryInput `json:"logs"`
}

type logEntryInput struct {
	ActionID  int64  `json:"action_id"`
	Seq       int    `json:"seq"`
	Path      string `json:"path"`
	Direction string `json:"direction"`
	Hash      string `json:"hash"`
	Size      int64  `json:"size_bytes"`
}

func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// --- 处理器 ---

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "task191-reproof 构建图可复现性隔离证明服务",
		"api":     "/api/health, /api/targets, /api/actions/{id}/declarations ...",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.app.Stats(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
