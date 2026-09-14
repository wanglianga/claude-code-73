package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ---------- 物品 ----------

type Item struct {
	ID             int        `json:"id"`
	ItemNo         string     `json:"item_no"`
	Category       string     `json:"category"`
	Description    string     `json:"description"`
	Features       string     `json:"features"`
	Photos         []string   `json:"photos"`
	FoundLineID    *int       `json:"found_line_id"`
	LineName       string     `json:"line_name"`
	FoundVehicleID *int       `json:"found_vehicle_id"`
	PlateNo        string     `json:"plate_no"`
	FoundStopID    *int       `json:"found_stop_id"`
	StopName       string     `json:"stop_name"`
	FoundAt        *time.Time `json:"found_at"`
	HandedByRole   string     `json:"handed_by_role"`
	HandedByName   string     `json:"handed_by_name"`
	StorageCabinet string     `json:"storage_cabinet"`
	ValueLevel     string     `json:"value_level"`
	SpecialType    string     `json:"special_type"`
	RetentionDays  int        `json:"retention_days"`
	RetentionUntil *time.Time `json:"retention_until"`
	StorageRules   string     `json:"storage_rules"`
	FleetID        *int       `json:"fleet_id"`
	FleetName      string     `json:"fleet_name"`
	Status         string     `json:"status"`
	MatchedReportID *int      `json:"matched_report_id"`
	CreatedAt      time.Time  `json:"created_at"`
}

const itemSelect = `
SELECT i.id, i.item_no, i.category, i.description, COALESCE(i.features,''), COALESCE(i.photos,'{}'),
 i.found_line_id, COALESCE(l.name,''), i.found_vehicle_id, COALESCE(v.plate_no,''),
 i.found_stop_id, COALESCE(st.name,''), i.found_at,
 COALESCE(i.handed_by_role,''), COALESCE(i.handed_by_name,''),
 COALESCE(i.storage_cabinet,''), i.value_level, i.special_type, i.retention_days, i.retention_until,
 COALESCE(i.storage_rules,''), i.fleet_id, COALESCE(f.name,''), i.status, i.matched_report_id, i.created_at
FROM found_items i
LEFT JOIN lines l ON l.id=i.found_line_id
LEFT JOIN vehicles v ON v.id=i.found_vehicle_id
LEFT JOIN stops st ON st.id=i.found_stop_id
LEFT JOIN fleets f ON f.id=i.fleet_id`

func scanItem(s scanner) (*Item, error) {
	it := &Item{}
	var lineID, vehID, stopID, fleetID, matchedReport sql.NullInt64
	var foundAt, retention sql.NullTime
	err := s.Scan(&it.ID, &it.ItemNo, &it.Category, &it.Description, &it.Features, pq.Array(&it.Photos),
		&lineID, &it.LineName, &vehID, &it.PlateNo, &stopID, &it.StopName, &foundAt,
		&it.HandedByRole, &it.HandedByName, &it.StorageCabinet, &it.ValueLevel, &it.SpecialType,
		&it.RetentionDays, &retention, &it.StorageRules, &fleetID, &it.FleetName, &it.Status, &matchedReport, &it.CreatedAt)
	if err != nil {
		return nil, err
	}
	set := func(p **int, v sql.NullInt64) {
		if v.Valid {
			n := int(v.Int64)
			*p = &n
		}
	}
	set(&it.FoundLineID, lineID)
	set(&it.FoundVehicleID, vehID)
	set(&it.FoundStopID, stopID)
	set(&it.FleetID, fleetID)
	set(&it.MatchedReportID, matchedReport)
	if foundAt.Valid {
		it.FoundAt = &foundAt.Time
	}
	if retention.Valid {
		it.RetentionUntil = &retention.Time
	}
	return it, nil
}

func getItemBrief(id int) (*Item, error) {
	return scanItem(db.QueryRow(itemSelect+` WHERE i.id=$1`, id))
}

// 特殊物品保管规则
func storageRules(category, special string, valueLevel string) (specialType string, days int, cabinet, rules string) {
	specialType = "无"
	days = 30
	cabinet = ""
	rules = "普通物品：保管期 30 天，逾期公告后捐赠或销毁。"
	switch {
	case category == "危险品" || special == "危险品":
		return "危险品", 3, "安保暂存区（不入柜）", "危险品：不入普通柜，立即移交安保暂存区并报警联动；3 日内移交公安机关处理。"
	case category == "银行卡" || special == "银行卡":
		return "银行卡", 15, "", "银行卡：立即电话提醒失主挂失；保管期 15 天；逾期剪角销毁并留存影像。"
	case category == "药品" || special == "药品":
		return "药品", 7, "冷藏柜-C1", "药品：冷藏柜 2-8℃ 保管；保管期 7 天；逾期按医疗废弃物规范销毁（双人执行）。"
	case category == "儿童物品" || special == "儿童证件":
		return "儿童证件", 90, "", "儿童证件：优先电话联系监护人并站内公告；保管期 90 天；逾期移交学校或公安机关。"
	case category == "证件":
		return "身份证件", 90, "", "身份证件：涉及个人隐私，信息脱敏展示，调阅需审批；保管期 90 天，逾期移交公安机关。"
	case valueLevel == "贵重" || category == "手机" || category == "钱包":
		return "无", 90, "", "贵重物品：双人核验入库，保险柜保管，联动安保确认；保管期 90 天，逾期移交公安机关。"
	}
	return
}

func nextItemNo() string {
	var seq int
	// 取当日前缀下编号最大值+1，避免与种子/历史数据撞号
	db.QueryRow(`SELECT COALESCE(MAX(CAST(substring(item_no from 12 for 4) AS int)),0)+1
		FROM found_items WHERE item_no LIKE 'WP'||to_char(CURRENT_DATE,'YYYYMMDD')||'-%'`).Scan(&seq)
	return fmt.Sprintf("WP%s-%04d", time.Now().Format("20060102"), seq)
}

// 司机/保洁上交（待站务登记）
func handleHandin(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "driver", "station") {
		errJSON(w, 403, "仅司机或站务可上交物品")
		return
	}
	var body struct {
		Category       string `json:"category"`
		Description    string `json:"description"`
		Features       string `json:"features"`
		FoundLineID    *int   `json:"found_line_id"`
		FoundVehicleID *int   `json:"found_vehicle_id"`
		FoundStopID    *int   `json:"found_stop_id"`
		FoundAt        string `json:"found_at"`
		HandedByRole   string `json:"handed_by_role"` // driver/cleaner
		HandedByName   string `json:"handed_by_name"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	if body.Category == "" || strings.TrimSpace(body.Description) == "" {
		errJSON(w, 400, "物品类别与描述为必填项")
		return
	}
	if body.HandedByRole == "" {
		body.HandedByRole = "driver"
	}
	if body.HandedByName == "" {
		body.HandedByName = u.Name
	}
	foundAt := parseDT(body.FoundAt)
	if foundAt == nil {
		now := time.Now()
		foundAt = &now
	}
	// 保管车队：优先车辆所属车队，其次司机所属车队
	var fleetID *int
	if body.FoundVehicleID != nil {
		var fid int
		if db.QueryRow(`SELECT fleet_id FROM vehicles WHERE id=$1`, *body.FoundVehicleID).Scan(&fid) == nil {
			fleetID = &fid
		}
	}
	if fleetID == nil {
		fleetID = u.FleetID
	}
	var id int
	err := db.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_stop_id,found_at,
		handed_by_role,handed_by_id,handed_by_name,fleet_id,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending_register') RETURNING id`,
		nextItemNo(), body.Category, body.Description, body.Features, body.FoundLineID, body.FoundVehicleID, body.FoundStopID, *foundAt,
		body.HandedByRole, u.ID, body.HandedByName, fleetID).Scan(&id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &id, "物品上交", fmt.Sprintf("%s 上交：%s，待站务登记入库", body.HandedByName, body.Description))
	audit(u, "item", id, "handin", "上交物品待登记: "+body.Description)
	writeJSON(w, 200, map[string]any{"id": id})
}

// 站务直接登记入库
func handleRegisterItem(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可登记入库")
		return
	}
	var body registerBody
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	if body.Category == "" || strings.TrimSpace(body.Description) == "" {
		errJSON(w, 400, "物品类别与描述为必填项")
		return
	}
	foundAt := parseDT(body.FoundAt)
	if foundAt == nil {
		now := time.Now()
		foundAt = &now
	}
	id, err := doRegister(u, nil, &body, *foundAt)
	if err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

// 站务完成待登记物品的入库
func handleCompleteRegister(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可登记入库")
		return
	}
	id := pathID(r)
	var status string
	var foundAt sql.NullTime
	if err := db.QueryRow(`SELECT status, found_at FROM found_items WHERE id=$1`, id).Scan(&status, &foundAt); err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	if status != "pending_register" {
		errJSON(w, 409, "该物品不在待登记状态")
		return
	}
	var body registerBody
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	fa := time.Now()
	if foundAt.Valid {
		fa = foundAt.Time
	}
	if _, err := doRegister(u, &id, &body, fa); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

type registerBody struct {
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	Features       string   `json:"features"`
	Photos         []string `json:"photos"`
	FoundLineID    *int     `json:"found_line_id"`
	FoundVehicleID *int     `json:"found_vehicle_id"`
	FoundStopID    *int     `json:"found_stop_id"`
	FoundAt        string   `json:"found_at"`
	HandedByRole   string   `json:"handed_by_role"`
	HandedByName   string   `json:"handed_by_name"`
	StorageCabinet string   `json:"storage_cabinet"`
	ValueLevel     string   `json:"value_level"`
	SpecialType    string   `json:"special_type"`
}

func doRegister(u *User, itemID *int, body *registerBody, foundAt time.Time) (int, error) {
	if body.ValueLevel == "" {
		body.ValueLevel = "普通"
	}
	photos := body.Photos
	if photos == nil {
		photos = []string{}
	}
	specialType, days, cabinetHint, rules := storageRules(body.Category, body.SpecialType, body.ValueLevel)
	cabinet := body.StorageCabinet
	if cabinet == "" {
		cabinet = cabinetHint
	}
	if cabinet == "" {
		return 0, fmt.Errorf("请指定存放柜")
	}
	retentionUntil := foundAt.AddDate(0, 0, days)
	var id int
	if itemID == nil {
		// 新登记
		var fleetID *int = u.FleetID
		if body.FoundVehicleID != nil {
			var fid int
			if db.QueryRow(`SELECT fleet_id FROM vehicles WHERE id=$1`, *body.FoundVehicleID).Scan(&fid) == nil {
				fleetID = &fid
			}
		}
		if body.HandedByRole == "" {
			body.HandedByRole = "driver"
		}
		err := db.QueryRow(`INSERT INTO found_items(item_no,category,description,features,photos,found_line_id,found_vehicle_id,found_stop_id,found_at,
			handed_by_role,handed_by_id,handed_by_name,storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'in_storage',$20) RETURNING id`,
			nextItemNo(), body.Category, body.Description, body.Features, pq.Array(photos),
			body.FoundLineID, body.FoundVehicleID, body.FoundStopID, foundAt,
			body.HandedByRole, nil, body.HandedByName, cabinet, body.ValueLevel, specialType, days, retentionUntil, rules, fleetID, u.ID).Scan(&id)
		if err != nil {
			return 0, err
		}
	} else {
		id = *itemID
		_, err := db.Exec(`UPDATE found_items SET category=$1, description=$2, features=$3, photos=$4,
			storage_cabinet=$5, value_level=$6, special_type=$7, retention_days=$8, retention_until=$9, storage_rules=$10,
			status='in_storage', registered_by=$11, updated_at=now() WHERE id=$12`,
			body.Category, body.Description, body.Features, pq.Array(photos),
			cabinet, body.ValueLevel, specialType, days, retentionUntil, rules, u.ID, id)
		if err != nil {
			return 0, err
		}
	}
	addEvent(u, nil, &id, "站务登记入库", fmt.Sprintf("存放柜 %s，贵重程度 %s，保管期 %d 天", cabinet, body.ValueLevel, days))
	audit(u, "item", id, "register", fmt.Sprintf("登记入库 柜=%s 贵重=%s 特殊=%s", cabinet, body.ValueLevel, specialType))
	// 触发报警联动
	if specialType == "危险品" {
		db.Exec(`INSERT INTO alarms(item_id,type,level,title) VALUES($1,'danger','紧急',$2)`, id, "危险品上交："+body.Description+"，已暂存安保区，请立即处置")
		audit(u, "item", id, "alarm", "触发危险品紧急报警")
	} else if body.ValueLevel == "贵重" {
		db.Exec(`INSERT INTO alarms(item_id,type,level,title) VALUES($1,'valuable','一般',$2)`, id, "贵重物品入库："+body.Description+"（柜 "+cabinet+"），请安保确认保管措施")
		audit(u, "item", id, "alarm", "触发贵重物品报警联动")
	}
	return id, nil
}

func handleListItems(w http.ResponseWriter, r *http.Request, u *User) {
	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if s := r.URL.Query().Get("status"); s != "" {
		add("i.status = $%d", s)
	}
	if s := r.URL.Query().Get("category"); s != "" {
		add("i.category = $%d", s)
	}
	if q := r.URL.Query().Get("q"); q != "" {
		add("(i.item_no ILIKE '%%'||$%[1]d||'%%' OR i.description ILIKE '%%'||$%[1]d||'%%')", q)
	}
	if r.URL.Query().Get("expiring") == "1" {
		where = append(where, "i.status='in_storage' AND i.retention_until <= CURRENT_DATE + 7")
	}
	rows, err := db.Query(itemSelect+` WHERE `+strings.Join(where, " AND ")+` ORDER BY i.id DESC LIMIT 300`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err == nil {
			list = append(list, it)
		}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func handleGetItem(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r)
	it, err := getItemBrief(id)
	if err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	out := map[string]any{"item": it, "events": queryEvents(nil, &id)}
	// 认领单
	claims := []map[string]any{}
	crows, _ := db.Query(`SELECT id, report_id, claimant_name, claimant_phone, COALESCE(claimant_id_card,''),
		COALESCE(verify_method,''), COALESCE(delegate_name,''), COALESCE(delegate_id_card,''), COALESCE(delegate_relation,''),
		COALESCE(match_notes,''), COALESCE(sign_photo,''), status, created_at, verified_at
		FROM claims WHERE item_id=$1 ORDER BY id DESC`, id)
	for crows.Next() {
		var cid, reportID sql.NullInt64
		var c struct {
			id                                                    int
			name, phone, idc, method, dname, didc, drel, notes, photo, status string
			created                                               time.Time
			verifiedAt                                            sql.NullTime
		}
		crows.Scan(&cid, &reportID, &c.name, &c.phone, &c.idc, &c.method, &c.dname, &c.didc, &c.drel, &c.notes, &c.photo, &c.status, &c.created, &c.verifiedAt)
		m := map[string]any{
			"id": cid.Int64, "claimant_name": c.name, "claimant_phone": c.phone,
			"claimant_id_card": maskIDCard(c.idc), "verify_method": c.method, "delegate_name": c.dname,
			"delegate_id_card": maskIDCard(c.didc), "delegate_relation": c.drel, "match_notes": c.notes,
			"sign_photo": c.photo, "status": c.status, "created_at": c.created,
		}
		if reportID.Valid {
			rid := int(reportID.Int64)
			m["report_id"] = rid
		}
		if c.verifiedAt.Valid {
			m["verified_at"] = c.verifiedAt.Time
		}
		claims = append(claims, m)
	}
	crows.Close()
	out["claims"] = claims
	// 移交记录
	transfers := []map[string]any{}
	trows, _ := db.Query(`SELECT t.id, f1.name, f2.name, COALESCE(t.reason,''), t.status, COALESCE(u1.name,''), COALESCE(u2.name,''), t.created_at, t.confirmed_at
		FROM transfers t JOIN fleets f1 ON f1.id=t.from_fleet_id JOIN fleets f2 ON f2.id=t.to_fleet_id
		LEFT JOIN users u1 ON u1.id=t.initiated_by LEFT JOIN users u2 ON u2.id=t.confirmed_by
		WHERE t.item_id=$1 ORDER BY t.id DESC`, id)
	for trows.Next() {
		var tid int
		var f1, f2, reason, status, u1, u2 string
		var created time.Time
		var confirmed sql.NullTime
		trows.Scan(&tid, &f1, &f2, &reason, &status, &u1, &u2, &created, &confirmed)
		m := map[string]any{"id": tid, "from_fleet": f1, "to_fleet": f2, "reason": reason, "status": status,
			"initiated_by": u1, "confirmed_by": u2, "created_at": created}
		if confirmed.Valid {
			m["confirmed_at"] = confirmed.Time
		}
		transfers = append(transfers, m)
	}
	trows.Close()
	out["transfers"] = transfers
	// 处置记录
	disposals := []map[string]any{}
	drows, _ := db.Query(`SELECT id, action, notice_done, contact_done, confirm_done, executed, COALESCE(notes,''), created_at, executed_at
		FROM disposals WHERE item_id=$1 ORDER BY id DESC`, id)
	for drows.Next() {
		var did int
		var action, notes string
		var notice, contact, confirm, executed bool
		var created time.Time
		var executedAt sql.NullTime
		drows.Scan(&did, &action, &notice, &contact, &confirm, &executed, &notes, &created, &executedAt)
		m := map[string]any{"id": did, "action": action, "notice_done": notice, "contact_done": contact,
			"confirm_done": confirm, "executed": executed, "notes": notes, "created_at": created}
		if executedAt.Valid {
			m["executed_at"] = executedAt.Time
		}
		disposals = append(disposals, m)
	}
	drows.Close()
	out["disposals"] = disposals
	// 关联报警
	alarms := []map[string]any{}
	arows, _ := db.Query(`SELECT id, type, level, title, status, created_at FROM alarms WHERE item_id=$1 ORDER BY id DESC`, id)
	for arows.Next() {
		var aid int
		var typ, level, title, status string
		var created time.Time
		arows.Scan(&aid, &typ, &level, &title, &status, &created)
		alarms = append(alarms, map[string]any{"id": aid, "type": typ, "level": level, "title": title, "status": status, "created_at": created})
	}
	arows.Close()
	out["alarms"] = alarms
	// 审计链
	out["audits"] = queryAudit(`WHERE (entity_type='item' AND entity_id=$1)
		OR (entity_type='claim' AND entity_id IN (SELECT id FROM claims WHERE item_id=$1))
		OR (entity_type='transfer' AND entity_id IN (SELECT id FROM transfers WHERE item_id=$1))
		OR (entity_type='disposal' AND entity_id IN (SELECT id FROM disposals WHERE item_id=$1))`, id)
	writeJSON(w, 200, out)
}

// ---------- 认领 ----------

func handleCreateClaim(w http.ResponseWriter, r *http.Request, u *User) {
	reportID := pathID(r)
	rep, err := scanReport(db.QueryRow(reportSelect+` WHERE r.id=$1`, reportID))
	if err != nil {
		errJSON(w, 404, "招领单不存在")
		return
	}
	if u.Role == "passenger" && (rep.PassengerID == nil || *rep.PassengerID != u.ID) {
		errJSON(w, 403, "只能对自己的招领单申请认领")
		return
	}
	if rep.Status != "matched" || rep.MatchedItemID == nil {
		errJSON(w, 409, "该招领单尚未匹配到物品")
		return
	}
	var body struct {
		ClaimantName    string `json:"claimant_name"`
		ClaimantPhone   string `json:"claimant_phone"`
		ClaimantIDCard  string `json:"claimant_id_card"`
		VerifyMethod    string `json:"verify_method"` // id_card/description/delegate
		DelegateName    string `json:"delegate_name"`
		DelegateIDCard  string `json:"delegate_id_card"`
		DelegateRelation string `json:"delegate_relation"`
		MatchNotes      string `json:"match_notes"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	if body.ClaimantName == "" || body.ClaimantPhone == "" {
		errJSON(w, 400, "认领人姓名与电话为必填项")
		return
	}
	if body.VerifyMethod == "id_card" && strings.TrimSpace(body.ClaimantIDCard) == "" {
		errJSON(w, 400, "身份证核验需填写认领人证件号")
		return
	}
	if body.VerifyMethod == "delegate" && (body.DelegateName == "" || body.DelegateIDCard == "") {
		errJSON(w, 400, "委托代领需填写被委托人姓名与证件号")
		return
	}
	// 证件号完整入库（供授权站务核验）；脱敏仅在接口读出时进行
	var claimID int
	err = db.QueryRow(`INSERT INTO claims(item_id,report_id,claimant_name,claimant_phone,claimant_id_card,verify_method,
		delegate_name,delegate_id_card,delegate_relation,match_notes,submitted_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		*rep.MatchedItemID, reportID, body.ClaimantName, body.ClaimantPhone, body.ClaimantIDCard, body.VerifyMethod,
		body.DelegateName, body.DelegateIDCard, body.DelegateRelation, body.MatchNotes, u.ID).Scan(&claimID)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	setReportStatus(reportID, "claim_verifying")
	addEvent(u, &reportID, rep.MatchedItemID, "申请认领", fmt.Sprintf("%s 申请认领，核验方式：%s，待站务核验", body.ClaimantName, verifyLabel(body.VerifyMethod)))
	audit(u, "claim", claimID, "create", fmt.Sprintf("认领申请（招领单 #%d，物品 #%d）", reportID, *rep.MatchedItemID))
	writeJSON(w, 200, map[string]any{"id": claimID})
}

func verifyLabel(m string) string {
	switch m {
	case "id_card":
		return "身份证核验"
	case "delegate":
		return "委托代领"
	default:
		return "描述匹配"
	}
}

func handleVerifyClaim(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可核验认领")
		return
	}
	id := pathID(r)
	var body struct {
		VerifyMethod string `json:"verify_method"`
		MatchNotes   string `json:"match_notes"`
	}
	readJSON(r, &body)
	var itemID, reportID sql.NullInt64
	err := db.QueryRow(`UPDATE claims SET status='verified', verify_method=COALESCE(NULLIF($1,''),verify_method),
		match_notes=COALESCE(NULLIF($2,''),match_notes), verified_by=$3, verified_at=now()
		WHERE id=$4 AND status='pending' RETURNING item_id, report_id`, body.VerifyMethod, body.MatchNotes, u.ID, id).Scan(&itemID, &reportID)
	if err != nil {
		errJSON(w, 409, "认领单不存在或不在待核验状态")
		return
	}
	addEvent(u, intPtr(reportID), intPtr(itemID), "认领核验通过", fmt.Sprintf("站务核验通过（%s），等待签收", verifyLabel(body.VerifyMethod)))
	audit(u, "claim", id, "verify", "核验通过: "+body.MatchNotes)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleRejectClaim(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可驳回认领")
		return
	}
	id := pathID(r)
	var body struct {
		Reason string `json:"reason"`
	}
	readJSON(r, &body)
	var itemID, reportID sql.NullInt64
	err := db.QueryRow(`UPDATE claims SET status='rejected', match_notes=$1 WHERE id=$2 AND status IN ('pending','verified') RETURNING item_id, report_id`, body.Reason, id).Scan(&itemID, &reportID)
	if err != nil {
		errJSON(w, 409, "认领单不存在或状态不允许驳回")
		return
	}
	if reportID.Valid {
		setReportStatus(int(reportID.Int64), "matched")
	}
	addEvent(u, intPtr(reportID), intPtr(itemID), "认领驳回", "认领申请被驳回："+body.Reason)
	audit(u, "claim", id, "reject", "驳回认领: "+body.Reason)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleSignClaim(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可办理签收")
		return
	}
	id := pathID(r)
	var body struct {
		SignPhoto string `json:"sign_photo"`
	}
	if err := readJSON(r, &body); err != nil || body.SignPhoto == "" {
		errJSON(w, 400, "请上传签收照片")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID, reportID sql.NullInt64
	err = tx.QueryRow(`UPDATE claims SET status='signed', sign_photo=$1 WHERE id=$2 AND status='verified' RETURNING item_id, report_id`, body.SignPhoto, id).Scan(&itemID, &reportID)
	if err != nil {
		errJSON(w, 409, "认领单不在待签收状态（需先核验通过）")
		return
	}
	if itemID.Valid {
		tx.Exec(`UPDATE found_items SET status='claimed', updated_at=now() WHERE id=$1`, itemID.Int64)
	}
	if reportID.Valid {
		tx.Exec(`UPDATE lost_reports SET status='claimed', next_role='', updated_at=now() WHERE id=$1`, reportID.Int64)
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, intPtr(reportID), intPtr(itemID), "签收完成", "认领人签收，物品出库，招领单完结")
	audit(u, "claim", id, "sign", "签收照片: "+body.SignPhoto)
	if itemID.Valid {
		audit(u, "item", int(itemID.Int64), "claimed", "物品已签收出库")
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func intPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

// ---------- 车队间移交 ----------

func handleCreateTransfer(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可发起车队移交")
		return
	}
	itemID := pathID(r)
	var body struct {
		ToFleetID int    `json:"to_fleet_id"`
		Reason    string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil || body.ToFleetID == 0 {
		errJSON(w, 400, "请选择目标车队")
		return
	}
	it, err := getItemBrief(itemID)
	if err != nil || it.FleetID == nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	if *it.FleetID == body.ToFleetID {
		errJSON(w, 400, "物品已在该车队")
		return
	}
	var id int
	err = db.QueryRow(`INSERT INTO transfers(item_id,from_fleet_id,to_fleet_id,reason,initiated_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		itemID, *it.FleetID, body.ToFleetID, body.Reason, u.ID).Scan(&id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &itemID, "发起车队移交", fmt.Sprintf("物品移交至目标车队，原因：%s，等待对方确认", body.Reason))
	audit(u, "transfer", id, "create", fmt.Sprintf("物品 #%d 车队移交发起", itemID))
	writeJSON(w, 200, map[string]any{"id": id})
}

func handleConfirmTransfer(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可确认接收")
		return
	}
	id := pathID(r)
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID, toFleet int
	err = tx.QueryRow(`UPDATE transfers SET status='confirmed', confirmed_by=$1, confirmed_at=now() WHERE id=$2 AND status='pending' RETURNING item_id, to_fleet_id`, u.ID, id).Scan(&itemID, &toFleet)
	if err != nil {
		errJSON(w, 409, "移交单不存在或已确认")
		return
	}
	tx.Exec(`UPDATE found_items SET fleet_id=$1, updated_at=now() WHERE id=$2`, toFleet, itemID)
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &itemID, "车队移交确认", "接收方站务确认接收，物品保管车队已变更")
	audit(u, "transfer", id, "confirm", "移交确认完成")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 逾期处置 ----------

// 物品类型 → 允许的处置方式
func allowedDisposalActions(it *Item) []string {
	switch it.SpecialType {
	case "药品":
		return []string{"destroy"}
	case "银行卡":
		return []string{"destroy"}
	case "危险品":
		return []string{"transfer_out"}
	case "儿童证件", "身份证件":
		return []string{"transfer_out"}
	}
	if it.ValueLevel == "贵重" || it.Category == "手机" || it.Category == "钱包" {
		return []string{"transfer_out"}
	}
	return []string{"donate", "destroy"}
}

func handleCreateDisposal(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可发起处置")
		return
	}
	itemID := pathID(r)
	it, err := getItemBrief(itemID)
	if err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	if it.Status != "expired" {
		errJSON(w, 409, "仅逾期物品可发起处置")
		return
	}
	var body struct {
		Action string `json:"action"` // transfer_out/destroy/donate
		Notes  string `json:"notes"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	allowed := false
	for _, a := range allowedDisposalActions(it) {
		if a == body.Action {
			allowed = true
		}
	}
	if !allowed {
		errJSON(w, 400, "该物品类型不允许此处置方式（按保管规则执行）")
		return
	}
	var id int
	err = db.QueryRow(`INSERT INTO disposals(item_id,action,notes) VALUES($1,$2,$3) RETURNING id`, itemID, body.Action, body.Notes).Scan(&id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &itemID, "发起逾期处置", fmt.Sprintf("处置方式：%s；需依次完成公告、联系、移交确认", disposalLabel(body.Action)))
	audit(u, "disposal", id, "create", "发起处置: "+disposalLabel(body.Action))
	writeJSON(w, 200, map[string]any{"id": id})
}

func disposalLabel(a string) string {
	switch a {
	case "transfer_out":
		return "移交公安机关"
	case "donate":
		return "捐赠"
	default:
		return "销毁"
	}
}

func handleDisposalStep(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可执行处置步骤")
		return
	}
	id := pathID(r)
	var body struct {
		Step string `json:"step"` // notice/contact/confirm
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	var query, label string
	switch body.Step {
	case "notice":
		query = `UPDATE disposals SET notice_done=TRUE, notice_at=now(), notice_by=$1 WHERE id=$2 AND notice_done=FALSE RETURNING item_id`
		label = "公告"
	case "contact":
		query = `UPDATE disposals SET contact_done=TRUE, contact_at=now(), contact_by=$1 WHERE id=$2 AND notice_done=TRUE AND contact_done=FALSE RETURNING item_id`
		label = "联系乘客"
	case "confirm":
		query = `UPDATE disposals SET confirm_done=TRUE, confirm_at=now(), confirm_by=$1 WHERE id=$2 AND contact_done=TRUE AND confirm_done=FALSE RETURNING item_id`
		label = "移交确认"
	default:
		errJSON(w, 400, "未知步骤")
		return
	}
	var itemID int
	if err := db.QueryRow(query, u.ID, id).Scan(&itemID); err != nil {
		errJSON(w, 409, "步骤状态不允许（需按 公告→联系→移交确认 顺序完成）")
		return
	}
	addEvent(u, nil, &itemID, "处置步骤", fmt.Sprintf("逾期处置：%s 已完成", label))
	audit(u, "disposal", id, "step_"+body.Step, label+" 完成")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleExecuteDisposal(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可执行处置")
		return
	}
	id := pathID(r)
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID int
	var action string
	err = tx.QueryRow(`UPDATE disposals SET executed=TRUE, executed_at=now(), executed_by=$1
		WHERE id=$2 AND notice_done AND contact_done AND confirm_done AND NOT executed
		RETURNING item_id, action`, u.ID, id).Scan(&itemID, &action)
	if err != nil {
		errJSON(w, 409, "需先完成 公告→联系→移交确认 三个步骤")
		return
	}
	status := map[string]string{"transfer_out": "transferred", "destroy": "destroyed", "donate": "donated"}[action]
	tx.Exec(`UPDATE found_items SET status=$1, updated_at=now() WHERE id=$2`, status, itemID)
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &itemID, "处置执行", fmt.Sprintf("逾期物品已%s，流程完结", disposalLabel(action)))
	audit(u, "disposal", id, "execute", "处置执行: "+disposalLabel(action))
	audit(u, "item", itemID, "dispose", "物品状态变更为 "+status)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 报警 ----------

func handleListAlarms(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security", "station", "cs", "dispatcher") {
		errJSON(w, 403, "无权查看报警")
		return
	}
	rows, err := db.Query(`SELECT a.id, a.item_id, COALESCE(i.item_no,''), a.type, a.level, a.title, a.status,
		COALESCE(hu.name,''), a.handled_at, COALESCE(a.handle_notes,''), a.created_at
		FROM alarms a LEFT JOIN found_items i ON i.id=a.item_id LEFT JOIN users hu ON hu.id=a.handled_by
		ORDER BY CASE a.status WHEN 'open' THEN 0 WHEN 'ack' THEN 1 ELSE 2 END, a.id DESC LIMIT 200`)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, itemID sql.NullInt64
		var itemNo, typ, level, title, status, handler, notes string
		var handledAt sql.NullTime
		var created time.Time
		rows.Scan(&id, &itemID, &itemNo, &typ, &level, &title, &status, &handler, &handledAt, &notes, &created)
		m := map[string]any{"id": id.Int64, "item_no": itemNo, "type": typ, "level": level, "title": title,
			"status": status, "handler": handler, "handle_notes": notes, "created_at": created}
		if itemID.Valid {
			m["item_id"] = itemID.Int64
		}
		if handledAt.Valid {
			m["handled_at"] = handledAt.Time
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"alarms": list})
}

func handleAckAlarm(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security") {
		errJSON(w, 403, "仅安保可处理报警")
		return
	}
	id := pathID(r)
	var itemID sql.NullInt64
	if err := db.QueryRow(`UPDATE alarms SET status='ack', handled_by=$1, handled_at=now() WHERE id=$2 AND status='open' RETURNING item_id`, u.ID, id).Scan(&itemID); err != nil {
		errJSON(w, 409, "报警不存在或已处理")
		return
	}
	addEvent(u, nil, intPtr(itemID), "报警确认", "安保已确认报警，跟进处置中")
	audit(u, "alarm", id, "ack", "安保确认报警")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCloseAlarm(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security") {
		errJSON(w, 403, "仅安保可关闭报警")
		return
	}
	id := pathID(r)
	var body struct {
		Notes string `json:"notes"`
	}
	readJSON(r, &body)
	var itemID sql.NullInt64
	if err := db.QueryRow(`UPDATE alarms SET status='closed', handled_by=$1, handled_at=now(), handle_notes=$2 WHERE id=$3 AND status IN ('open','ack') RETURNING item_id`, u.ID, body.Notes, id).Scan(&itemID); err != nil {
		errJSON(w, 409, "报警不存在或已关闭")
		return
	}
	addEvent(u, nil, intPtr(itemID), "报警关闭", "报警处置完成："+body.Notes)
	audit(u, "alarm", id, "close", "关闭报警: "+body.Notes)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 工作台 / 审计 / 事件 ----------

func queryEvents(reportID, itemID *int) []map[string]any {
	var rows *sql.Rows
	var err error
	if reportID != nil {
		rows, err = db.Query(`SELECT actor_role, actor_name, action, COALESCE(detail,''), created_at FROM events WHERE report_id=$1 ORDER BY id`, *reportID)
	} else {
		rows, err = db.Query(`SELECT actor_role, actor_name, action, COALESCE(detail,''), created_at FROM events WHERE item_id=$1 ORDER BY id`, *itemID)
	}
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var role, name, action, detail string
		var ts time.Time
		rows.Scan(&role, &name, &action, &detail, &ts)
		list = append(list, map[string]any{"actor_role": role, "actor_name": name, "action": action, "detail": detail, "created_at": ts})
	}
	return list
}

func queryAudit(where string, args ...any) []map[string]any {
	rows, err := db.Query(`SELECT entity_type, entity_id, actor_role, actor_name, action, COALESCE(detail,''), created_at
		FROM audit_logs `+where+` ORDER BY id DESC LIMIT 300`, args...)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var et, role, name, action, detail string
		var eid int
		var ts time.Time
		rows.Scan(&et, &eid, &role, &name, &action, &detail, &ts)
		list = append(list, map[string]any{"entity_type": et, "entity_id": eid, "actor_role": role, "actor_name": name,
			"action": action, "detail": detail, "created_at": ts})
	}
	return list
}

func handleAudit(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "security", "cs", "station", "dispatcher") {
		errJSON(w, 403, "无权查看审计日志")
		return
	}
	et := r.URL.Query().Get("entity_type")
	eid := qInt(r, "entity_id")
	if et != "" && eid != nil {
		writeJSON(w, 200, map[string]any{"audits": queryAudit(`WHERE entity_type=$1 AND entity_id=$2`, et, *eid)})
		return
	}
	writeJSON(w, 200, map[string]any{"audits": queryAudit("")})
}

func handleDashboard(w http.ResponseWriter, r *http.Request, u *User) {
	out := map[string]any{"role": u.Role}
	count := func(q string, args ...any) int {
		var n int
		db.QueryRow(q, args...).Scan(&n)
		return n
	}
	out["counts"] = map[string]int{
		"reports_submitted":      count(`SELECT count(*) FROM lost_reports WHERE status='submitted'`),
		"reports_searching":      count(`SELECT count(*) FROM lost_reports WHERE status='searching'`),
		"reports_matched":        count(`SELECT count(*) FROM lost_reports WHERE status='matched'`),
		"reports_claim_verifying": count(`SELECT count(*) FROM lost_reports WHERE status='claim_verifying'`),
		"items_in_storage":       count(`SELECT count(*) FROM found_items WHERE status='in_storage'`),
		"items_pending_register": count(`SELECT count(*) FROM found_items WHERE status='pending_register'`),
		"items_expired":          count(`SELECT count(*) FROM found_items WHERE status='expired'`),
		"items_expiring":         count(`SELECT count(*) FROM found_items WHERE status='in_storage' AND retention_until <= CURRENT_DATE + 7`),
		"claims_pending":         count(`SELECT count(*) FROM claims WHERE status='pending'`),
		"claims_verified":        count(`SELECT count(*) FROM claims WHERE status='verified'`),
		"surveillance_pending":   count(`SELECT count(*) FROM surveillance_requests WHERE status='pending'`),
		"alarms_open":            count(`SELECT count(*) FROM alarms WHERE status IN ('open','ack')`),
		"transfers_pending":      count(`SELECT count(*) FROM transfers WHERE status='pending'`),
	}
	// 乘客：我的招领单
	if u.Role == "passenger" {
		rows, _ := db.Query(reportSelect+` WHERE r.passenger_id=$1 ORDER BY r.id DESC LIMIT 50`, u.ID)
		list := []*Report{}
		for rows.Next() {
			if rep, err := scanReport(rows); err == nil {
				list = append(list, rep)
			}
		}
		rows.Close()
		out["my_reports"] = list
	}
	// 客服：待受理 + 进行中（含下一步责任人）
	if u.Role == "cs" || u.Role == "admin" {
		rows, _ := db.Query(reportSelect+` WHERE r.status IN ('submitted','searching','matched','claim_verifying') ORDER BY r.id DESC LIMIT 100`)
		list := []*Report{}
		for rows.Next() {
			if rep, err := scanReport(rows); err == nil {
				list = append(list, rep)
			}
		}
		rows.Close()
		out["active_reports"] = list
	}
	// 司机：我的上交
	if u.Role == "driver" {
		rows, _ := db.Query(itemSelect+` WHERE i.handed_by_id=$1 ORDER BY i.id DESC LIMIT 50`, u.ID)
		list := []*Item{}
		for rows.Next() {
			if it, err := scanItem(rows); err == nil {
				list = append(list, it)
			}
		}
		rows.Close()
		out["my_handins"] = list
	}
	// 站务：待登记 / 逾期 / 即将到期 / 待核验认领 / 待确认移交
	if u.Role == "station" || u.Role == "admin" {
		fetch := func(where string) []*Item {
			rows, _ := db.Query(itemSelect+` WHERE `+where+` ORDER BY i.id DESC LIMIT 100`)
			list := []*Item{}
			for rows.Next() {
				if it, err := scanItem(rows); err == nil {
					list = append(list, it)
				}
			}
			rows.Close()
			return list
		}
		out["items_pending_register"] = fetch(`i.status='pending_register'`)
		out["items_expired"] = fetch(`i.status='expired'`)
		out["items_expiring"] = fetch(`i.status='in_storage' AND i.retention_until <= CURRENT_DATE + 7`)
		// 待核验认领
		crows, _ := db.Query(`SELECT c.id, c.item_id, i.item_no, i.description, c.claimant_name, c.claimant_phone, c.status, c.created_at
			FROM claims c JOIN found_items i ON i.id=c.item_id WHERE c.status IN ('pending','verified') ORDER BY c.id DESC LIMIT 50`)
		claims := []map[string]any{}
		for crows.Next() {
			var cid, itemID int
			var itemNo, desc, name, phone, status string
			var created time.Time
			crows.Scan(&cid, &itemID, &itemNo, &desc, &name, &phone, &status, &created)
			claims = append(claims, map[string]any{"id": cid, "item_id": itemID, "item_no": itemNo, "item_description": desc,
				"claimant_name": name, "claimant_phone": phone, "status": status, "created_at": created})
		}
		crows.Close()
		out["claims_todo"] = claims
		// 待确认移交（发到本车队）
		if u.FleetID != nil {
			trows, _ := db.Query(`SELECT t.id, t.item_id, i.item_no, i.description, f.name, t.created_at
				FROM transfers t JOIN found_items i ON i.id=t.item_id JOIN fleets f ON f.id=t.from_fleet_id
				WHERE t.status='pending' AND t.to_fleet_id=$1`, *u.FleetID)
			transfers := []map[string]any{}
			for trows.Next() {
				var tid, itemID int
				var itemNo, desc, fromFleet string
				var created time.Time
				trows.Scan(&tid, &itemID, &itemNo, &desc, &fromFleet, &created)
				transfers = append(transfers, map[string]any{"id": tid, "item_id": itemID, "item_no": itemNo,
					"item_description": desc, "from_fleet": fromFleet, "created_at": created})
			}
			trows.Close()
			out["transfers_todo"] = transfers
		}
	}
	// 调度：排查中的招领单
	if u.Role == "dispatcher" || u.Role == "admin" {
		rows, _ := db.Query(reportSelect+` WHERE r.status='searching' ORDER BY r.id DESC LIMIT 100`)
		list := []*Report{}
		for rows.Next() {
			if rep, err := scanReport(rows); err == nil {
				list = append(list, rep)
			}
		}
		rows.Close()
		out["searching_reports"] = list
	}
	// 安保：待审批调阅 + 未关闭报警
	if u.Role == "security" || u.Role == "admin" {
		out["surveillance_pending_list"] = querySurveillance(`WHERE s.status='pending'`)
	}
	writeJSON(w, 200, out)
}

// ---------- 上传 ----------

func handleUpload(w http.ResponseWriter, r *http.Request, u *User) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		errJSON(w, 400, "文件过大或格式错误（最大 8MB）")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		errJSON(w, 400, "缺少文件")
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	allow := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	if !allow[ext] {
		errJSON(w, 400, "仅支持 jpg/png/webp/gif 图片")
		return
	}
	if err := os.MkdirAll(uploadDir(), 0o755); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	name := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), strings.ReplaceAll(u.Username, "/", "_"), ext)
	path := filepath.Join(uploadDir(), name)
	out, err := os.Create(path)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	audit(u, "upload", 0, "upload", "上传图片 /uploads/"+name)
	writeJSON(w, 200, map[string]string{"url": "/uploads/" + name})
}
