package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var (
	db        *sql.DB
	jwtSecret []byte
	// 门店本地时区：全系统统一按 Asia/Shanghai 解析与展示时间
	appLoc = time.UTC
)

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Phone    string `json:"phone,omitempty"`
	FleetID  *int   `json:"fleet_id,omitempty"`
}

// ---------- 工具 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func qInt(r *http.Request, key string) *int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return &n
	}
	return nil
}

func maskIDCard(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 7 {
		return "***"
	}
	return string(r[:3]) + strings.Repeat("*", len(r)-7) + string(r[len(r)-4:])
}

// ---------- 简易 HMAC Token ----------

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func makeToken(uid int) string {
	payload := fmt.Sprintf(`{"uid":%d,"exp":%d}`, uid, time.Now().Add(24*time.Hour).Unix())
	p := b64([]byte(payload))
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(p))
	return p + "." + b64(mac.Sum(nil))
}

func parseToken(tok string) (int, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return 0, false
	}
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(parts[0]))
	expected := b64(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	var p struct {
		UID int   `json:"uid"`
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &p) != nil || time.Now().Unix() > p.Exp {
		return 0, false
	}
	return p.UID, true
}

// ---------- 鉴权中间件 ----------

type Handler func(http.ResponseWriter, *http.Request, *User)

func currentUser(r *http.Request) *User {
	c, err := r.Cookie("token")
	if err != nil || c.Value == "" {
		return nil
	}
	uid, ok := parseToken(c.Value)
	if !ok {
		return nil
	}
	u := &User{}
	var phone sql.NullString
	var fleetID sql.NullInt64
	err = db.QueryRow(`SELECT id, username, role, name, COALESCE(phone,''), fleet_id FROM users WHERE id=$1`, uid).
		Scan(&u.ID, &u.Username, &u.Role, &u.Name, &phone, &fleetID)
	if err != nil {
		return nil
	}
	u.Phone = phone.String
	if fleetID.Valid {
		n := int(fleetID.Int64)
		u.FleetID = &n
	}
	return u
}

func authed(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			errJSON(w, 401, "未登录或登录已过期")
			return
		}
		h(w, r, u)
	}
}

func requireRole(u *User, roles ...string) bool {
	for _, r := range roles {
		if u.Role == r || u.Role == "admin" {
			return true
		}
	}
	return false
}

func pathID(r *http.Request) int {
	id, _ := strconv.Atoi(r.PathValue("id"))
	return id
}

// ---------- 审计与事件 ----------

func audit(actor *User, entityType string, entityID int, action, detail string) {
	var uid *int
	role, name := "system", "系统"
	if actor != nil {
		uid = &actor.ID
		role, name = actor.Role, actor.Name
	}
	_, err := db.Exec(`INSERT INTO audit_logs(entity_type,entity_id,actor_id,actor_role,actor_name,action,detail) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		entityType, entityID, uid, role, name, action, detail)
	if err != nil {
		log.Printf("audit error: %v", err)
	}
}

func addEvent(actor *User, reportID, itemID *int, action, detail string) {
	var uid *int
	role, name := "system", "系统"
	if actor != nil {
		uid = &actor.ID
		role, name = actor.Role, actor.Name
	}
	_, err := db.Exec(`INSERT INTO events(report_id,item_id,actor_id,actor_role,actor_name,action,detail) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		reportID, itemID, uid, role, name, action, detail)
	if err != nil {
		log.Printf("event error: %v", err)
	}
}

// 招领单状态 → 下一步责任角色
var reportNextRole = map[string]string{
	"submitted":       "cs",
	"searching":       "dispatcher",
	"matched":         "passenger",
	"claim_verifying": "station",
	"claimed":         "",
	"closed_unfound":  "",
}

func setReportStatus(reportID int, status string) {
	db.Exec(`UPDATE lost_reports SET status=$1, next_role=$2, updated_at=now() WHERE id=$3`, status, reportNextRole[status], reportID)
}

// ---------- 主入口 ----------

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("dev-insecure-secret")
	}
	db = mustConnect()
	defer db.Close()
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		appLoc = loc
		time.Local = loc // 种子数据、巡检等所有本地时间统一门店时区
	}
	migrate()
	seed()
	go expirySweeper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			errJSON(w, 503, "db not ready")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// 认证
	mux.HandleFunc("POST /api/auth/login", handleLogin)
	mux.HandleFunc("POST /api/auth/logout", handleLogout)
	mux.HandleFunc("GET /api/auth/me", authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		writeJSON(w, 200, map[string]any{"user": u})
	}))

	// 基础数据
	mux.HandleFunc("GET /api/meta", authed(handleMeta))

	// 招领单
	mux.HandleFunc("POST /api/reports", authed(handleCreateReport))
	mux.HandleFunc("GET /api/reports", authed(handleListReports))
	mux.HandleFunc("GET /api/reports/{id}", authed(handleGetReport))
	mux.HandleFunc("POST /api/reports/{id}/accept", authed(handleAcceptReport))
	mux.HandleFunc("POST /api/reports/{id}/reply", authed(handleReplyReport))
	mux.HandleFunc("POST /api/reports/{id}/dispatch-note", authed(handleDispatchNote))
	mux.HandleFunc("GET /api/reports/{id}/candidates", authed(handleCandidates))
	mux.HandleFunc("POST /api/reports/{id}/match", authed(handleMatchItem))
	mux.HandleFunc("POST /api/reports/{id}/close", authed(handleCloseReport))
	mux.HandleFunc("POST /api/reports/{id}/claim", authed(handleCreateClaim))
	mux.HandleFunc("GET /api/reports/{id}/credential", authed(handleReportCredential))

	// 监控调阅
	mux.HandleFunc("POST /api/reports/{id}/surveillance", authed(handleCreateSurveillance))
	mux.HandleFunc("GET /api/surveillance", authed(handleListSurveillance))
	mux.HandleFunc("POST /api/surveillance/{id}/approve", authed(handleApproveSurveillance))
	mux.HandleFunc("POST /api/surveillance/{id}/reject", authed(handleRejectSurveillance))
	mux.HandleFunc("POST /api/surveillance/{id}/result", authed(handleSurveillanceResult))

	// 物品
	mux.HandleFunc("POST /api/handin", authed(handleHandin)) // 司机/保洁上交
	mux.HandleFunc("POST /api/items", authed(handleRegisterItem))
	mux.HandleFunc("POST /api/items/{id}/register", authed(handleCompleteRegister))
	mux.HandleFunc("GET /api/items", authed(handleListItems))
	mux.HandleFunc("GET /api/items/{id}", authed(handleGetItem))
	mux.HandleFunc("GET /api/items/{id}/valuable", authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		handleItemValuable(w, r, u, pathID(r))
	}))
	mux.HandleFunc("GET /api/items/{id}/sensitive", authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		handleSensitiveView(w, r, u, pathID(r))
	}))
	mux.HandleFunc("POST /api/items/{id}/transfer", authed(handleCreateTransfer))
	mux.HandleFunc("POST /api/items/{id}/dispose", authed(handleCreateDisposal))

	// 认领
	mux.HandleFunc("POST /api/claims/{id}/verify", authed(handleVerifyClaim))
	mux.HandleFunc("POST /api/claims/{id}/reject", authed(handleRejectClaim))
	mux.HandleFunc("POST /api/claims/{id}/sign", authed(handleSignClaim))
	mux.HandleFunc("GET /api/claims/{id}/credential", authed(handleClaimCredential))

	// 移交 / 处置 / 报警
	mux.HandleFunc("POST /api/transfers/{id}/confirm", authed(handleConfirmTransfer))
	mux.HandleFunc("POST /api/disposals/{id}/step", authed(handleDisposalStep))
	mux.HandleFunc("POST /api/disposals/{id}/execute", authed(handleExecuteDisposal))
	mux.HandleFunc("GET /api/alarms", authed(handleListAlarms))
	mux.HandleFunc("POST /api/alarms/{id}/ack", authed(handleAckAlarm))
	mux.HandleFunc("POST /api/alarms/{id}/close", authed(handleCloseAlarm))

	// 贵重物品双人入柜
	mux.HandleFunc("POST /api/valuable/intakes", authed(handleValuableIntakeCreate))
	mux.HandleFunc("GET /api/valuable/intakes", authed(handleValuableIntakeList))
	mux.HandleFunc("POST /api/valuable/intakes/{id}/countersign", authed(handleValuableCountersign))

	// 敏感信息授权查看（客服认领前仅见必要描述）
	mux.HandleFunc("POST /api/items/{id}/sensitive/request", authed(handleSensitiveRequest))
	mux.HandleFunc("GET /api/sensitive/requests", authed(handleSensitiveList))
	mux.HandleFunc("POST /api/sensitive/{id}/approve", authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		handleSensitiveApprove(w, r, u, true)
	}))
	mux.HandleFunc("POST /api/sensitive/{id}/reject", authed(func(w http.ResponseWriter, r *http.Request, u *User) {
		handleSensitiveApprove(w, r, u, false)
	}))

	// 换班交接 / 主管复核
	mux.HandleFunc("GET /api/handovers/my-vault", authed(handleMyVault))
	mux.HandleFunc("POST /api/handovers", authed(handleHandoverCreate))
	mux.HandleFunc("GET /api/handovers", authed(handleHandoverList))
	mux.HandleFunc("GET /api/handovers/{id}", authed(handleHandoverGet))
	mux.HandleFunc("POST /api/handovers/{id}/check", authed(handleHandoverCheck))
	mux.HandleFunc("GET /api/reviews", authed(handleReviewList))
	mux.HandleFunc("POST /api/reviews/{id}/resolve", authed(handleReviewResolve))

	// 工作台 / 审计
	mux.HandleFunc("GET /api/dashboard", authed(handleDashboard))
	mux.HandleFunc("GET /api/audit", authed(handleAudit))

	// 上传与静态
	mux.HandleFunc("POST /api/upload", authed(handleUpload))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir()))))

	log.Printf("api listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, logMiddleware(mux)))
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func uploadDir() string {
	d := os.Getenv("UPLOAD_DIR")
	if d == "" {
		d = "/data/uploads"
	}
	return d
}

// 逾期巡检：到期未认领物品进入 expired
func expirySweeper() {
	sweep := func() {
		rows, err := db.Query(`SELECT id, item_no FROM found_items WHERE retention_until < CURRENT_DATE AND status IN ('in_storage','matched')`)
		if err != nil {
			return
		}
		type it struct {
			id  int
			no  string
		}
		var list []it
		for rows.Next() {
			var x it
			rows.Scan(&x.id, &x.no)
			list = append(list, x)
		}
		rows.Close()
		for _, x := range list {
			db.Exec(`UPDATE found_items SET status='expired', updated_at=now() WHERE id=$1`, x.id)
			addEvent(nil, nil, &x.id, "逾期", fmt.Sprintf("物品 %s 超过保管期限，进入逾期待处置状态", x.no))
			audit(nil, "item", x.id, "expire", "保管期满，系统自动标记逾期")
		}
	}
	sweep()
	for range time.Tick(30 * time.Minute) {
		sweep()
	}
}
