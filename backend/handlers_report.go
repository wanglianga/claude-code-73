package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------- 认证 ----------

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	u := &User{}
	var hash string
	var fleetID sql.NullInt64
	err := db.QueryRow(`SELECT id, username, password_hash, role, name, COALESCE(phone,''), fleet_id FROM users WHERE username=$1`,
		strings.TrimSpace(body.Username)).
		Scan(&u.ID, &u.Username, &hash, &u.Role, &u.Name, &u.Phone, &fleetID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		errJSON(w, 401, "用户名或密码错误")
		return
	}
	if fleetID.Valid {
		n := int(fleetID.Int64)
		u.FleetID = &n
	}
	http.SetCookie(w, &http.Cookie{
		Name: "token", Value: makeToken(u.ID), Path: "/",
		HttpOnly: true, MaxAge: 86400, SameSite: http.SameSiteLaxMode,
	})
	audit(u, "user", u.ID, "login", "登录系统")
	writeJSON(w, 200, map[string]any{"user": u})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 基础数据 ----------

func handleMeta(w http.ResponseWriter, r *http.Request, u *User) {
	type Stop struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Seq  int    `json:"seq"`
	}
	type Line struct {
		ID      int    `json:"id"`
		Code    string `json:"code"`
		Name    string `json:"name"`
		FleetID int    `json:"fleet_id"`
		Stops   []Stop `json:"stops"`
	}
	type Vehicle struct {
		ID      int    `json:"id"`
		PlateNo string `json:"plate_no"`
		LineID  int    `json:"line_id"`
		FleetID int    `json:"fleet_id"`
	}
	type Fleet struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	lines := []Line{}
	lrows, err := db.Query(`SELECT id, code, name, fleet_id FROM lines ORDER BY code`)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	lineIdx := map[int]int{}
	for lrows.Next() {
		var l Line
		lrows.Scan(&l.ID, &l.Code, &l.Name, &l.FleetID)
		l.Stops = []Stop{}
		lineIdx[l.ID] = len(lines)
		lines = append(lines, l)
	}
	lrows.Close()
	srows, _ := db.Query(`SELECT id, line_id, name, seq FROM stops ORDER BY line_id, seq`)
	for srows.Next() {
		var s Stop
		var lid int
		srows.Scan(&s.ID, &lid, &s.Name, &s.Seq)
		if i, ok := lineIdx[lid]; ok {
			lines[i].Stops = append(lines[i].Stops, s)
		}
	}
	srows.Close()
	vehicles := []Vehicle{}
	vrows, _ := db.Query(`SELECT id, plate_no, line_id, fleet_id FROM vehicles ORDER BY plate_no`)
	for vrows.Next() {
		var v Vehicle
		vrows.Scan(&v.ID, &v.PlateNo, &v.LineID, &v.FleetID)
		vehicles = append(vehicles, v)
	}
	vrows.Close()
	fleets := []Fleet{}
	frows, _ := db.Query(`SELECT id, name FROM fleets ORDER BY id`)
	for frows.Next() {
		var f Fleet
		frows.Scan(&f.ID, &f.Name)
		fleets = append(fleets, f)
	}
	frows.Close()
	writeJSON(w, 200, map[string]any{
		"lines":    lines,
		"vehicles": vehicles,
		"fleets":   fleets,
		"categories": []string{"手机", "钱包", "证件", "背包", "儿童物品", "银行卡", "药品", "危险品", "其他"},
		"cabinets": []string{"A-01", "A-02", "A-03", "B-01", "B-02", "B-03", "C-01", "D-11", "D-12", "冷藏柜-C1", "安保暂存区（不入柜）"},
	})
}

// ---------- 招领单 ----------

type Report struct {
	ID               int        `json:"id"`
	ReportNo         string     `json:"report_no"`
	PassengerID      *int       `json:"passenger_id,omitempty"`
	Category         string     `json:"category"`
	Description      string     `json:"description"`
	Features         string     `json:"features"`
	LineID           *int       `json:"line_id"`
	LineName         string     `json:"line_name"`
	VehicleID        *int       `json:"vehicle_id"`
	PlateNo          string     `json:"plate_no"`
	VehicleUnknown   bool       `json:"vehicle_unknown"`
	BoardStopID      *int       `json:"board_stop_id"`
	BoardStop        string     `json:"board_stop"`
	AlightStopID     *int       `json:"alight_stop_id"`
	AlightStop       string     `json:"alight_stop"`
	RideStart        *time.Time `json:"ride_start"`
	RideEnd          *time.Time `json:"ride_end"`
	SeatPosition     string     `json:"seat_position"`
	IsTransfer       bool       `json:"is_transfer"`
	TransferLineID   *int       `json:"transfer_line_id"`
	TransferLineName string     `json:"transfer_line_name"`
	TransferStopID   *int       `json:"transfer_stop_id"`
	TransferStop     string     `json:"transfer_stop"`
	ContactName      string     `json:"contact_name"`
	ContactPhone     string     `json:"contact_phone"`
	PassengerIDCard  string     `json:"passenger_id_card"`
	Status           string     `json:"status"`
	NextRole         string     `json:"next_role"`
	MatchedItemID    *int       `json:"matched_item_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

const reportSelect = `
SELECT r.id, r.report_no, r.passenger_id, r.category, r.description, COALESCE(r.features,''),
 r.line_id, COALESCE(l.name,''), r.vehicle_id, COALESCE(v.plate_no,''), r.vehicle_unknown,
 r.board_stop_id, COALESCE(bs.name,''), r.alight_stop_id, COALESCE(al.name,''),
 r.ride_start, r.ride_end, COALESCE(r.seat_position,''),
 r.is_transfer, r.transfer_line_id, COALESCE(tl.name,''), r.transfer_stop_id, COALESCE(tst.name,''),
 r.contact_name, r.contact_phone, COALESCE(r.passenger_id_card,''),
 r.status, r.next_role, r.matched_item_id, r.created_at
FROM lost_reports r
LEFT JOIN lines l ON l.id=r.line_id
LEFT JOIN vehicles v ON v.id=r.vehicle_id
LEFT JOIN stops bs ON bs.id=r.board_stop_id
LEFT JOIN stops al ON al.id=r.alight_stop_id
LEFT JOIN lines tl ON tl.id=r.transfer_line_id
LEFT JOIN stops tst ON tst.id=r.transfer_stop_id`

type scanner interface{ Scan(dest ...any) error }

func scanReport(s scanner) (*Report, error) {
	r := &Report{}
	var passengerID, lineID, vehicleID, boardID, alightID, tLineID, tStopID, matchedID sql.NullInt64
	var rideStart, rideEnd sql.NullTime
	err := s.Scan(&r.ID, &r.ReportNo, &passengerID, &r.Category, &r.Description, &r.Features,
		&lineID, &r.LineName, &vehicleID, &r.PlateNo, &r.VehicleUnknown,
		&boardID, &r.BoardStop, &alightID, &r.AlightStop,
		&rideStart, &rideEnd, &r.SeatPosition,
		&r.IsTransfer, &tLineID, &r.TransferLineName, &tStopID, &r.TransferStop,
		&r.ContactName, &r.ContactPhone, &r.PassengerIDCard,
		&r.Status, &r.NextRole, &matchedID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	set := func(p **int, v sql.NullInt64) {
		if v.Valid {
			n := int(v.Int64)
			*p = &n
		}
	}
	set(&r.PassengerID, passengerID)
	set(&r.LineID, lineID)
	set(&r.VehicleID, vehicleID)
	set(&r.BoardStopID, boardID)
	set(&r.AlightStopID, alightID)
	set(&r.TransferLineID, tLineID)
	set(&r.TransferStopID, tStopID)
	set(&r.MatchedItemID, matchedID)
	if rideStart.Valid {
		r.RideStart = &rideStart.Time
	}
	if rideEnd.Valid {
		r.RideEnd = &rideEnd.Time
	}
	r.PassengerIDCard = maskIDCard(r.PassengerIDCard) // 身份证件脱敏
	return r, nil
}

func parseDT(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// 浏览器 datetime-local 不带时区，一律按门店本地时区（Asia/Shanghai）解释
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02 15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, appLoc); err == nil {
			return &t
		}
	}
	return nil
}

func handleCreateReport(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "passenger", "cs") {
		errJSON(w, 403, "仅乘客或客服可提交遗失申报")
		return
	}
	var body struct {
		Category        string `json:"category"`
		Description     string `json:"description"`
		Features        string `json:"features"`
		LineID          *int   `json:"line_id"`
		VehicleID       *int   `json:"vehicle_id"`
		VehicleUnknown  bool   `json:"vehicle_unknown"`
		BoardStopID     *int   `json:"board_stop_id"`
		AlightStopID    *int   `json:"alight_stop_id"`
		RideStart       string `json:"ride_start"`
		RideEnd         string `json:"ride_end"`
		SeatPosition    string `json:"seat_position"`
		IsTransfer      bool   `json:"is_transfer"`
		TransferLineID  *int   `json:"transfer_line_id"`
		TransferStopID  *int   `json:"transfer_stop_id"`
		ContactName     string `json:"contact_name"`
		ContactPhone    string `json:"contact_phone"`
		PassengerIDCard string `json:"passenger_id_card"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	if body.Category == "" || strings.TrimSpace(body.Description) == "" || body.ContactName == "" || body.ContactPhone == "" {
		errJSON(w, 400, "物品类别、描述、联系人、联系电话为必填项")
		return
	}
	var seq int
	db.QueryRow(`SELECT COALESCE(MAX(CAST(substring(report_no from 12 for 4) AS int)),0)+1
		FROM lost_reports WHERE report_no LIKE 'LP'||to_char(CURRENT_DATE,'YYYYMMDD')||'-%'`).Scan(&seq)
	reportNo := fmt.Sprintf("LP%s-%04d", time.Now().Format("20060102"), seq)
	passengerID := &u.ID
	if u.Role == "cs" {
		passengerID = nil // 客服代录，不绑定乘客账号
	}
	var id int
	err := db.QueryRow(`INSERT INTO lost_reports(report_no,passenger_id,category,description,features,line_id,vehicle_id,vehicle_unknown,
		board_stop_id,alight_stop_id,ride_start,ride_end,seat_position,is_transfer,transfer_line_id,transfer_stop_id,
		contact_name,contact_phone,passenger_id_card,status,next_role)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'submitted','cs') RETURNING id`,
		reportNo, passengerID, body.Category, body.Description, body.Features, body.LineID, body.VehicleID, body.VehicleUnknown,
		body.BoardStopID, body.AlightStopID, parseDT(body.RideStart), parseDT(body.RideEnd), body.SeatPosition,
		body.IsTransfer, body.TransferLineID, body.TransferStopID,
		body.ContactName, body.ContactPhone, body.PassengerIDCard).Scan(&id)
	if err != nil {
		errJSON(w, 500, "创建失败: "+err.Error())
		return
	}
	addEvent(u, &id, nil, "乘客申报", fmt.Sprintf("申报遗失：%s（%s）", body.Description, body.Category))
	audit(u, "report", id, "create", "创建招领单 "+reportNo)
	writeJSON(w, 200, map[string]any{"id": id, "report_no": reportNo})
}

func handleListReports(w http.ResponseWriter, r *http.Request, u *User) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if u.Role == "passenger" {
		add("r.passenger_id = $%d", u.ID)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		add("r.status = $%d", s)
	}
	if s := r.URL.Query().Get("next_role"); s != "" {
		add("r.next_role = $%d", s)
	}
	if q := r.URL.Query().Get("q"); q != "" {
		add("(r.report_no ILIKE '%%'||$%[1]d||'%%' OR r.description ILIKE '%%'||$%[1]d||'%%')", q)
	}
	rows, err := db.Query(reportSelect+` WHERE `+strings.Join(where, " AND ")+` ORDER BY r.id DESC LIMIT 200`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []*Report{}
	for rows.Next() {
		rep, err := scanReport(rows)
		if err == nil {
			list = append(list, rep)
		}
	}
	writeJSON(w, 200, map[string]any{"reports": list})
}

func getReportFull(id int) (map[string]any, error) {
	rep, err := scanReport(db.QueryRow(reportSelect+` WHERE r.id=$1`, id))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"report": rep}
	out["events"] = queryEvents(&id, nil)
	// 匹配的物品
	if rep.MatchedItemID != nil {
		if item, err := getItemBrief(*rep.MatchedItemID); err == nil {
			out["matched_item"] = item
		}
	}
	// 认领单
	claims := []map[string]any{}
	crows, _ := db.Query(`SELECT id, item_id, claimant_name, claimant_phone, COALESCE(claimant_id_card,''),
		COALESCE(verify_method,''), COALESCE(delegate_name,''), COALESCE(delegate_id_card,''), COALESCE(delegate_relation,''),
		COALESCE(match_notes,''), COALESCE(sign_photo,''), status, created_at, verified_at
		FROM claims WHERE report_id=$1 ORDER BY id DESC`, id)
	for crows.Next() {
		var c struct {
			id, itemID                                   int
			name, phone, idc, method, dname, didc, drel, notes, photo, status string
			created                                    time.Time
			verifiedAt                                 sql.NullTime
		}
		crows.Scan(&c.id, &c.itemID, &c.name, &c.phone, &c.idc, &c.method, &c.dname, &c.didc, &c.drel, &c.notes, &c.photo, &c.status, &c.created, &c.verifiedAt)
		m := map[string]any{
			"id": c.id, "item_id": c.itemID, "claimant_name": c.name, "claimant_phone": c.phone,
			"claimant_id_card": maskIDCard(c.idc), "verify_method": c.method, "delegate_name": c.dname,
			"delegate_id_card": maskIDCard(c.didc), "delegate_relation": c.drel, "match_notes": c.notes,
			"sign_photo": c.photo, "status": c.status, "created_at": c.created,
		}
		if c.verifiedAt.Valid {
			m["verified_at"] = c.verifiedAt.Time
		}
		claims = append(claims, m)
	}
	crows.Close()
	out["claims"] = claims
	// 监控调阅
	out["surveillance"] = querySurveillance(`WHERE s.report_id=$1`, id)
	// 审计链（本单 + 匹配物品）
	auditRows := []map[string]any{}
	arows, _ := db.Query(`SELECT actor_role, actor_name, action, detail, created_at FROM audit_logs
		WHERE (entity_type='report' AND entity_id=$1) OR (entity_type='item' AND entity_id=$2)
		   OR (entity_type='claim' AND entity_id IN (SELECT id FROM claims WHERE report_id=$1))
		   OR (entity_type='surveillance' AND entity_id IN (SELECT id FROM surveillance_requests WHERE report_id=$1))
		ORDER BY id`, id, rep.MatchedItemID)
	for arows.Next() {
		var role, name, action, detail string
		var ts time.Time
		arows.Scan(&role, &name, &action, &detail, &ts)
		auditRows = append(auditRows, map[string]any{"actor_role": role, "actor_name": name, "action": action, "detail": detail, "created_at": ts})
	}
	arows.Close()
	out["audits"] = auditRows
	return out, nil
}

func handleGetReport(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r)
	out, err := getReportFull(id)
	if err != nil {
		errJSON(w, 404, "招领单不存在")
		return
	}
	// 乘客只能看自己的
	if u.Role == "passenger" {
		rep := out["report"].(*Report)
		if rep.PassengerID == nil || *rep.PassengerID != u.ID {
			errJSON(w, 403, "无权查看该招领单")
			return
		}
	}
	writeJSON(w, 200, out)
}

func handleAcceptReport(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "cs") {
		errJSON(w, 403, "仅客服可受理")
		return
	}
	id := pathID(r)
	res, err := db.Exec(`UPDATE lost_reports SET status='searching', next_role='dispatcher', assigned_cs_id=$1, updated_at=now() WHERE id=$2 AND status='submitted'`, u.ID, id)
	if err != nil || affected(res) == 0 {
		errJSON(w, 409, "该单不在待受理状态")
		return
	}
	addEvent(u, &id, nil, "客服受理", "已受理并登记，转调度排查")
	audit(u, "report", id, "accept", "客服受理，转调度排查")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleReplyReport(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "cs") {
		errJSON(w, 403, "仅客服可答复")
		return
	}
	id := pathID(r)
	var body struct {
		Message string `json:"message"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		errJSON(w, 400, "答复内容不能为空")
		return
	}
	addEvent(u, &id, nil, "客服答复", body.Message)
	audit(u, "report", id, "reply", "答复乘客: "+body.Message)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleDispatchNote(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "dispatcher") {
		errJSON(w, 403, "仅调度可填写排查记录")
		return
	}
	id := pathID(r)
	var body struct {
		Note      string `json:"note"`
		VehicleID *int   `json:"vehicle_id"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Note) == "" {
		errJSON(w, 400, "排查记录不能为空")
		return
	}
	if body.VehicleID != nil {
		db.Exec(`UPDATE lost_reports SET vehicle_id=$1, vehicle_unknown=FALSE, updated_at=now() WHERE id=$2`, *body.VehicleID, id)
	}
	addEvent(u, &id, nil, "调度排查", body.Note)
	audit(u, "report", id, "dispatch_note", body.Note)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// 调度排查工具：按线路+时间段(+站点)从 GPS/刷卡记录找候选车辆
func handleCandidates(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "dispatcher", "cs", "security") {
		errJSON(w, 403, "无权使用排查工具")
		return
	}
	lineID := qInt(r, "line_id")
	start, end := parseDT(r.URL.Query().Get("start")), parseDT(r.URL.Query().Get("end"))
	if lineID == nil || start == nil || end == nil {
		errJSON(w, 400, "需要 line_id、start、end 参数")
		return
	}
	stopCond := ""
	args := []any{*lineID, *start, *end}
	if sid := qInt(r, "stop_id"); sid != nil {
		args = append(args, *sid)
		stopCond = " AND p.stop_id = $4"
	}
	rows, err := db.Query(`
		SELECT v.id, v.plate_no, count(p.id) AS pings,
		  (SELECT count(*) FROM card_swipes s WHERE s.vehicle_id=v.id AND s.ts BETWEEN $2 AND $3) AS swipes,
		  (SELECT string_agg(DISTINCT st.name, '、') FROM gps_pings p2 JOIN stops st ON st.id=p2.stop_id
		    WHERE p2.vehicle_id=v.id AND p2.ts BETWEEN $2 AND $3) AS stops
		FROM gps_pings p JOIN vehicles v ON v.id=p.vehicle_id
		WHERE v.line_id=$1 AND p.ts BETWEEN $2 AND $3`+stopCond+`
		GROUP BY v.id, v.plate_no ORDER BY pings DESC`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, pings, swipes int
		var plate string
		var stops sql.NullString
		rows.Scan(&id, &plate, &pings, &swipes, &stops)
		list = append(list, map[string]any{
			"vehicle_id": id, "plate_no": plate, "gps_pings": pings, "card_swipes": swipes,
			"stops": stops.String,
		})
	}
	// 同时给出当班司机
	type shift struct {
		DriverName string `json:"driver_name"`
		PlateNo    string `json:"plate_no"`
		Start      string `json:"start_time"`
		End        string `json:"end_time"`
	}
	shifts := []shift{}
	srows, _ := db.Query(`SELECT u.name, v.plate_no, s.start_time, s.end_time FROM driver_shifts s
		JOIN users u ON u.id=s.driver_id JOIN vehicles v ON v.id=s.vehicle_id
		WHERE v.line_id=$1 AND s.shift_date=$2::date`, *lineID, start.Format("2006-01-02"))
	for srows.Next() {
		var s shift
		srows.Scan(&s.DriverName, &s.PlateNo, &s.Start, &s.End)
		shifts = append(shifts, s)
	}
	srows.Close()
	writeJSON(w, 200, map[string]any{"candidates": list, "shifts": shifts})
}

func handleMatchItem(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "cs", "station") {
		errJSON(w, 403, "仅客服或站务可匹配物品")
		return
	}
	id := pathID(r)
	var body struct {
		ItemID int `json:"item_id"`
	}
	if err := readJSON(r, &body); err != nil || body.ItemID == 0 {
		errJSON(w, 400, "缺少 item_id")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemStatus, itemNo string
	if err := tx.QueryRow(`SELECT status, item_no FROM found_items WHERE id=$1 FOR UPDATE`, body.ItemID).Scan(&itemStatus, &itemNo); err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	if itemStatus != "in_storage" {
		errJSON(w, 409, "该物品当前状态不可匹配（需在库）")
		return
	}
	if _, err := tx.Exec(`UPDATE lost_reports SET matched_item_id=$1, status='matched', next_role='passenger', updated_at=now() WHERE id=$2`, body.ItemID, id); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE found_items SET status='matched', matched_report_id=$1, updated_at=now() WHERE id=$2`, id, body.ItemID); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, &id, &body.ItemID, "物品匹配", fmt.Sprintf("招领单与物品 %s 匹配成功，通知乘客申请认领", itemNo))
	audit(u, "report", id, "match", "匹配物品 "+itemNo)
	audit(u, "item", body.ItemID, "match", fmt.Sprintf("匹配招领单 #%d", id))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCloseReport(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "cs") {
		errJSON(w, 403, "仅客服可结案")
		return
	}
	id := pathID(r)
	var body struct {
		Reason string `json:"reason"`
	}
	readJSON(r, &body)
	if body.Reason == "" {
		body.Reason = "多方查找未果，与乘客确认后结案"
	}
	setReportStatus(id, "closed_unfound")
	addEvent(u, &id, nil, "结案", body.Reason)
	audit(u, "report", id, "close", body.Reason)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 监控调阅 ----------

func querySurveillance(where string, args ...any) []map[string]any {
	rows, err := db.Query(`SELECT s.id, s.report_id, s.scope_type, COALESCE(v.plate_no,''), COALESCE(st.name,''),
		s.time_start, s.time_end, s.reason, s.involves_privacy, s.status,
		COALESCE(ru.name,''), COALESCE(au.name,''), s.approved_at, COALESCE(s.reject_reason,''), COALESCE(s.result_notes,''), s.created_at
		FROM surveillance_requests s
		LEFT JOIN vehicles v ON v.id=s.vehicle_id
		LEFT JOIN stops st ON st.id=s.stop_id
		LEFT JOIN users ru ON ru.id=s.requester_id
		LEFT JOIN users au ON au.id=s.approver_id `+where+` ORDER BY s.id DESC`, args...)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, reportID int
		var scope, plate, stop, reason, status, reqName, appName, reject, result string
		var privacy bool
		var ts, te, created time.Time
		var approvedAt sql.NullTime
		rows.Scan(&id, &reportID, &scope, &plate, &stop, &ts, &te, &reason, &privacy, &status, &reqName, &appName, &approvedAt, &reject, &result, &created)
		m := map[string]any{
			"id": id, "report_id": reportID, "scope_type": scope, "plate_no": plate, "stop_name": stop,
			"time_start": ts, "time_end": te, "reason": reason, "involves_privacy": privacy,
			"status": status, "requester": reqName, "approver": appName, "reject_reason": reject,
			"result_notes": result, "created_at": created,
		}
		if approvedAt.Valid {
			m["approved_at"] = approvedAt.Time
		}
		list = append(list, m)
	}
	return list
}

func handleCreateSurveillance(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "dispatcher") {
		errJSON(w, 403, "仅调度可申请监控调阅")
		return
	}
	reportID := pathID(r)
	var body struct {
		ScopeType       string `json:"scope_type"`
		VehicleID       *int   `json:"vehicle_id"`
		StopID          *int   `json:"stop_id"`
		TimeStart       string `json:"time_start"`
		TimeEnd         string `json:"time_end"`
		Reason          string `json:"reason"`
		InvolvesPrivacy bool   `json:"involves_privacy"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	ts, te := parseDT(body.TimeStart), parseDT(body.TimeEnd)
	if body.Reason == "" || ts == nil || te == nil || (body.ScopeType != "vehicle" && body.ScopeType != "station") {
		errJSON(w, 400, "请填写调阅范围、时间段与理由")
		return
	}
	var id int
	err := db.QueryRow(`INSERT INTO surveillance_requests(report_id,requester_id,scope_type,vehicle_id,stop_id,time_start,time_end,reason,involves_privacy)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		reportID, u.ID, body.ScopeType, body.VehicleID, body.StopID, *ts, *te, body.Reason, body.InvolvesPrivacy).Scan(&id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, &reportID, nil, "申请监控调阅", fmt.Sprintf("申请调阅 %s 监控（%s ~ %s），待安保审批", scopeLabel(body.ScopeType), ts.Format("01-02 15:04"), te.Format("01-02 15:04")))
	audit(u, "surveillance", id, "create", fmt.Sprintf("创建调阅申请（招领单 #%d）", reportID))
	writeJSON(w, 200, map[string]any{"id": id})
}

func scopeLabel(s string) string {
	if s == "vehicle" {
		return "车载"
	}
	return "站点"
}

func handleListSurveillance(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security", "dispatcher", "cs", "station") {
		errJSON(w, 403, "无权查看")
		return
	}
	if u.Role == "dispatcher" {
		writeJSON(w, 200, map[string]any{"requests": querySurveillance(`WHERE s.requester_id=$1`, u.ID)})
		return
	}
	if s := r.URL.Query().Get("status"); s != "" {
		writeJSON(w, 200, map[string]any{"requests": querySurveillance(`WHERE s.status=$1`, s)})
		return
	}
	writeJSON(w, 200, map[string]any{"requests": querySurveillance("")})
}

func handleApproveSurveillance(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security") {
		errJSON(w, 403, "仅安保可审批监控调阅")
		return
	}
	id := pathID(r)
	var reportID int
	err := db.QueryRow(`UPDATE surveillance_requests SET status='approved', approver_id=$1, approved_at=now() WHERE id=$2 AND status='pending' RETURNING report_id`, u.ID, id).Scan(&reportID)
	if err != nil {
		errJSON(w, 409, "该申请不在待审批状态")
		return
	}
	addEvent(u, &reportID, nil, "监控调阅获批", "安保批准监控调阅申请，调度可查看并填写调阅结论")
	audit(u, "surveillance", id, "approve", "批准调阅")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleRejectSurveillance(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security") {
		errJSON(w, 403, "仅安保可审批监控调阅")
		return
	}
	id := pathID(r)
	var body struct {
		Reason string `json:"reason"`
	}
	readJSON(r, &body)
	var reportID int
	err := db.QueryRow(`UPDATE surveillance_requests SET status='rejected', approver_id=$1, approved_at=now(), reject_reason=$2 WHERE id=$3 AND status='pending' RETURNING report_id`, u.ID, body.Reason, id).Scan(&reportID)
	if err != nil {
		errJSON(w, 409, "该申请不在待审批状态")
		return
	}
	addEvent(u, &reportID, nil, "监控调阅被拒", "安保驳回调阅申请："+body.Reason)
	audit(u, "surveillance", id, "reject", "驳回调阅: "+body.Reason)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleSurveillanceResult(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "dispatcher") {
		errJSON(w, 403, "仅调度可填写调阅结论")
		return
	}
	id := pathID(r)
	var body struct {
		Result string `json:"result"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Result) == "" {
		errJSON(w, 400, "结论不能为空")
		return
	}
	var reportID int
	err := db.QueryRow(`UPDATE surveillance_requests SET result_notes=$1 WHERE id=$2 AND status='approved' RETURNING report_id`, body.Result, id).Scan(&reportID)
	if err != nil {
		errJSON(w, 409, "仅已批准的申请可填写结论")
		return
	}
	addEvent(u, &reportID, nil, "监控调阅结论", body.Result)
	audit(u, "surveillance", id, "result", body.Result)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func affected(res sql.Result) int64 {
	n, _ := res.RowsAffected()
	return n
}
