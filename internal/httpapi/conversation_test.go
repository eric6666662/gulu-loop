package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gululoop/internal/config"
	"gululoop/internal/dal"

	"github.com/jmoiron/sqlx"
)

// newConversationTestServer 真库 + 真路由；返回服务地址与 DB（便于预置数据）。
func newConversationTestServer(t *testing.T) (*httptest.Server, *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := dal.Open(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dal.Migrate(ctx, db); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	srv := httptest.NewServer(New(&config.Config{}, db).Handler())
	t.Cleanup(srv.Close)
	return srv, db
}

// TestCreateConversation 新建会话：空请求体 → 空标题、0 条消息、lastMessageTime 为 null。
func TestCreateConversation(t *testing.T) {
	srv, _ := newConversationTestServer(t)

	code, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/conversations", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] == "" || got["id"] == nil {
		t.Fatalf("应返回 id: %v", got)
	}
	if got["title"] != "" {
		t.Fatalf("新会话标题应为空串，实际 %v", got["title"])
	}
	if got["messageCount"].(float64) != 0 {
		t.Fatalf("messageCount 应为 0，实际 %v", got["messageCount"])
	}
	if got["lastMessageTime"] != nil {
		t.Fatalf("lastMessageTime 应为 null，实际 %v", got["lastMessageTime"])
	}
}

// TestListConversations 分页 + 按 last_message_time 倒序 + total。
func TestListConversations(t *testing.T) {
	srv, db := newConversationTestServer(t)

	// 三条：b 最新、a 次之、c 无消息（NULL 排最后）
	seed := []struct{ id, lmt string }{
		{"a", "2026-10-01 10:00:00"},
		{"b", "2026-10-02 10:00:00"},
	}
	for _, s := range seed {
		if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count,last_message_time,created_at,updated_at)
			VALUES (?,?,?,?,?,?)`, s.id, "会话"+s.id, 3, s.lmt, s.lmt, s.lmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c','空会话',0)`); err != nil {
		t.Fatal(err)
	}

	code, env := doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations?page=1&pageSize=2", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var page struct {
		List     []map[string]any `json:"list"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
		Total    int64            `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.Page != 1 || page.PageSize != 2 || len(page.List) != 2 {
		t.Fatalf("分页信息不符: %+v", page)
	}
	if page.List[0]["id"] != "b" || page.List[1]["id"] != "a" {
		t.Fatalf("应按 lastMessageTime 倒序: %v", page.List)
	}

	// 第二页拿到没有消息的会话（NULL 沉底）
	_, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations?page=2&pageSize=2", "")
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 1 || page.List[0]["id"] != "c" {
		t.Fatalf("第二页应只有 c: %v", page.List)
	}

	// 空库 → data.list 为 []
	srv2, _ := newConversationTestServer(t)
	_, env = doJSON(t, http.MethodGet, srv2.URL+"/api/v1/conversations", "")
	var empty struct {
		List []map[string]any `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &empty); err != nil {
		t.Fatal(err)
	}
	if empty.List == nil || len(empty.List) != 0 {
		t.Fatalf("空库应返回 []，实际 %s", env.Data)
	}
}

// TestRenameConversation 重命名：成功 / 空标题 400 / 不存在 404。
func TestRenameConversation(t *testing.T) {
	srv, _ := newConversationTestServer(t)

	_, env := doJSON(t, http.MethodPost, srv.URL+"/api/v1/conversations", "")
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatal(err)
	}

	code, env := doJSON(t, http.MethodPatch, srv.URL+"/api/v1/conversations/"+created.ID, `{"title":"  快速排序  "}`)
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	var renamed map[string]any
	_ = json.Unmarshal(env.Data, &renamed)
	if renamed["title"] != "快速排序" {
		t.Fatalf("标题应去掉两端空白，实际 %v", renamed["title"])
	}

	if code, env = doJSON(t, http.MethodPatch, srv.URL+"/api/v1/conversations/"+created.ID, `{"title":"   "}`); code != http.StatusBadRequest {
		t.Fatalf("空标题应为 400，实际 %d %+v", code, env)
	}
	if code, env = doJSON(t, http.MethodPatch, srv.URL+"/api/v1/conversations/nope", `{"title":"x"}`); code != http.StatusNotFound {
		t.Fatalf("不存在的会话应为 404，实际 %d %+v", code, env)
	}
}

// TestDeleteConversationCascade 删除会话级联清掉消息与摘要；重复删 → 404。
func TestDeleteConversationCascade(t *testing.T) {
	srv, db := newConversationTestServer(t)

	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content) VALUES ('m1','c1',1,'user','hi')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_conversation_summaries (id,conversation_id,content) VALUES ('s1','c1','sum')`); err != nil {
		t.Fatal(err)
	}

	code, env := doJSON(t, http.MethodDelete, srv.URL+"/api/v1/conversations/c1", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	if len(strings.TrimSpace(string(env.Data))) != 0 {
		t.Fatalf("删除成功时 data 应省略: %s", env.Data)
	}

	for _, q := range []struct {
		sql  string
		want int
	}{
		{`SELECT COUNT(*) FROM x_conversations WHERE id='c1'`, 0},
		{`SELECT COUNT(*) FROM x_messages WHERE conversation_id='c1'`, 0},
		{`SELECT COUNT(*) FROM x_conversation_summaries WHERE conversation_id='c1'`, 0},
	} {
		var n int
		if err := db.Get(&n, q.sql); err != nil {
			t.Fatal(err)
		}
		if n != q.want {
			t.Fatalf("%s = %d，期望 %d", q.sql, n, q.want)
		}
	}

	if code, env = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/conversations/c1", ""); code != http.StatusNotFound {
		t.Fatalf("重复删除应为 404，实际 %d %+v", code, env)
	}
}

// TestExportConversation 导出 Markdown：含标题、条数、用户/助手小节与模型落款。
func TestExportConversation(t *testing.T) {
	srv, db := newConversationTestServer(t)

	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','快速排序的 Go 泛型版本',2)`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id, role, content, model string
		seq                      int64
	}{
		{"m1", "user", "帮我把快速排序改成 Go 泛型版", "", 1},
		{"m2", "assistant", "泛型版要把元素类型和比较函数一起参数化：\n\n```go\nfunc QuickSort[T any](s []T) []T { return s }\n```", "kimi-k2", 2},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content,model) VALUES (?,?,?,?,?,?)`,
			r.id, "c1", r.seq, r.role, r.content, r.model); err != nil {
			t.Fatal(err)
		}
	}

	code, env := doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/c1/export", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}

	var got struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Filename != "快速排序的 Go 泛型版本.md" {
		t.Fatalf("文件名不符: %q", got.Filename)
	}
	for _, want := range []string{
		"# 快速排序的 Go 泛型版本",
		"2 条消息",
		"## 用户",
		"## 助手（kimi-k2）",
		"func QuickSort[T any]",
	} {
		if !strings.Contains(got.Content, want) {
			t.Fatalf("导出内容缺少 %q：\n%s", want, got.Content)
		}
	}

	// 不存在的会话 → 404
	if code, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/nope/export", ""); code != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d %+v", code, env)
	}
}

// TestExportFilenameFallback 标题为空 → 用会话 id 前 8 位；非法字符替换成下划线。
func TestExportFilenameFallback(t *testing.T) {
	srv, db := newConversationTestServer(t)

	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('abcdef12-3456-7890-abcd-ef1234567890','',0)`); err != nil {
		t.Fatal(err)
	}
	_, env := doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/abcdef12-3456-7890-abcd-ef1234567890/export", "")
	var got struct {
		Filename string `json:"filename"`
	}
	_ = json.Unmarshal(env.Data, &got)
	if got.Filename != "会话-abcdef12.md" {
		t.Fatalf("空标题文件名不符: %q", got.Filename)
	}

	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c2','a/b:c*d?e"f<g>h|i',0)`); err != nil {
		t.Fatal(err)
	}
	_, env = doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/c2/export", "")
	_ = json.Unmarshal(env.Data, &got)
	if strings.ContainsAny(strings.TrimSuffix(got.Filename, ".md"), `/\:*?"<>|`) {
		t.Fatalf("文件名仍含非法字符: %q", got.Filename)
	}
}

// TestGetMessageContent 单条消息内容：返回 {content}；不存在 → 404。
func TestGetMessageContent(t *testing.T) {
	srv, db := newConversationTestServer(t)

	if _, err := db.Exec(`INSERT INTO x_conversations (id,title,message_count) VALUES ('c1','t',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO x_messages (id,conversation_id,seq,role,content) VALUES ('m1','c1',1,'assistant','纯文本内容')`); err != nil {
		t.Fatal(err)
	}

	code, env := doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/c1/messages/m1", "")
	if code != http.StatusOK {
		t.Fatalf("应为 200，实际 %d %+v", code, env)
	}
	var got struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Content != "纯文本内容" {
		t.Fatalf("内容不符: %q", got.Content)
	}

	if code, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/c1/messages/nope", ""); code != http.StatusNotFound {
		t.Fatalf("应为 404，实际 %d", code)
	}
	// 跨会话也取不到
	if code, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/conversations/other/messages/m1", ""); code != http.StatusNotFound {
		t.Fatalf("跨会话应为 404，实际 %d", code)
	}
}
