package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"task191-reproof/internal/model"
	"task191-reproof/internal/service"
	"task191-reproof/internal/store"
)

// newTestServer 构造一个绑定到临时 SQLite 的真实 HTTP 服务器。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := filepath.Join(t.TempDir(), "httpapi.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(New(service.New(st)).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// doLogs 向 /api/actions/{id}/logs 投递一批日志，返回响应状态码与解码后的错误信息。
func doLogs(t *testing.T, baseURL string, actionID int64, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/actions/"+itoa(actionID)+"/logs", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out["error"]
}

func itoa(i int64) string {
	// 避免 strconv 依赖；测试内使用。
	return string(rune('0'+i)) // 仅用于 1..9 范围的测试 ID
}

// 创建目标+动作，返回动作 ID。
func setupAction(t *testing.T, baseURL string) int64 {
	t.Helper()
	tgtBody := mustJSON(t, map[string]any{"name": "demo", "description": ""})
	resp := do(t, http.MethodPost, baseURL+"/api/targets", tgtBody)
	defer resp.Body.Close()
	var tgt struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tgt); err != nil {
		t.Fatalf("decode target: %v", err)
	}
	actBody := mustJSON(t, map[string]any{"name": "a", "command": "cmd"})
	resp2 := do(t, http.MethodPost, baseURL+"/api/targets/"+itoa(tgt.ID)+"/actions", actBody)
	defer resp2.Body.Close()
	var act struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&act); err != nil {
		t.Fatalf("decode action: %v", err)
	}
	return act.ID
}

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestInvalidDirectionReturns400 验证非法访问方向返回 400 输入错误而非 500。
func TestInvalidDirectionReturns400(t *testing.T) {
	srv := newTestServer(t)
	actionID := setupAction(t, srv.URL)

	// 方向在 HTTP 层会先 strings.ToLower 归一化，故合法值仅 "read"/"write"；
	// 以下取值在归一化后仍非法，应作为输入错误返回 400。
	cases := []string{"sideways", "bad", "readwrite", "execute", ""}
	for _, dir := range cases {
		body := mustJSON(t, map[string]any{
			"logs": []map[string]any{
				{"action_id": actionID, "seq": 1, "path": "p", "direction": dir, "hash": "h", "size_bytes": 4},
			},
		})
		code, msg := doLogs(t, srv.URL, actionID, body)
		if code != http.StatusBadRequest {
			t.Fatalf("方向 %q 应返回 400，得到 %d (msg=%q)", dir, code, msg)
		}
	}
}

// TestValidDirectionWritesLog 验证合法方向（read/write）日志写入正常工作，不被错误修复误伤。
func TestValidDirectionWritesLog(t *testing.T) {
	srv := newTestServer(t)
	actionID := setupAction(t, srv.URL)

	body := mustJSON(t, map[string]any{
		"logs": []map[string]any{
			{"action_id": actionID, "seq": 1, "path": "in.txt", "direction": string(model.DirRead), "hash": "h-in", "size_bytes": 8},
			{"action_id": actionID, "seq": 2, "path": "out.bin", "direction": string(model.DirWrite), "hash": "h-out", "size_bytes": 16},
		},
	})
	code, _ := doLogs(t, srv.URL, actionID, body)
	if code != http.StatusCreated {
		t.Fatalf("合法方向应返回 201，得到 %d", code)
	}

	// 校验日志已持久化。
	resp := do(t, http.MethodGet, srv.URL+"/api/actions/"+itoa(actionID)+"/logs", "")
	defer resp.Body.Close()
	var logs []*model.AccessLog
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("期望持久化 2 条日志，得到 %d", len(logs))
	}
}

// TestConflictLogReturns409 验证同 (action,seq) 内容冲突返回 409 冲突而非 500。
func TestConflictLogReturns409(t *testing.T) {
	srv := newTestServer(t)
	actionID := setupAction(t, srv.URL)

	first := mustJSON(t, map[string]any{
		"logs": []map[string]any{
			{"action_id": actionID, "seq": 1, "path": "p", "direction": string(model.DirRead), "hash": "h1", "size_bytes": 4},
		},
	})
	if code, _ := doLogs(t, srv.URL, actionID, first); code != http.StatusCreated {
		t.Fatalf("首次写入应 201，得到 %d", code)
	}

	conflict := mustJSON(t, map[string]any{
		"logs": []map[string]any{
			{"action_id": actionID, "seq": 1, "path": "p", "direction": string(model.DirRead), "hash": "h2-DIFF", "size_bytes": 4},
		},
	})
	code, _ := doLogs(t, srv.URL, actionID, conflict)
	if code != http.StatusConflict {
		t.Fatalf("冲突日志应返回 409，得到 %d", code)
	}
}

// 确保上下文导入不被裁剪（保留以供未来扩展使用）。
var _ = context.Background
