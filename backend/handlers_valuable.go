package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ============ 贵重物品双人入柜 / 敏感信息授权 / 换班交接 / 主管复核 ============

// 手机、钱包或标记为贵重的物品必须走双人入柜流程
func isValuableCategory(category, valueLevel string) bool {
	return valueLevel == "贵重" || category == "手机" || category == "钱包"
}

// 校验并返回会签安保（仅安保本人或 admin 可完成会签；admin 可代指定安保账号）
func resolveSecurityCountersigner(u *User, bodySecurityID *int) (int, string, error) {
	if u.Role == "security" {
		return u.ID, u.Name, nil
	}
	if u.Role == "admin" && bodySecurityID != nil && *bodySecurityID > 0 {
		var srole, sname string
		if err := db.QueryRow(`SELECT role, name FROM users WHERE id=$1`, *bodySecurityID).Scan(&srole, &sname); err != nil {
			return 0, "", fmt.Errorf("会签安保不存在")
		}
		if srole != "security" {
			return 0, "", fmt.Errorf("会签方必须是安保角色")
		}
		return *bodySecurityID, sname, nil
	}
	return 0, "", fmt.Errorf("需安保登录会签（站务与安保双人共同确认）")
}

func nextIntakeNo() string {
	var seq int
	db.QueryRow(`SELECT COALESCE(MAX(CAST(substring(intake_no from 12 for 4) AS int)),0)+1
		FROM valuable_intakes WHERE intake_no LIKE 'VG'||to_char(CURRENT_DATE,'YYYYMMDD')||'-%'`).Scan(&seq)
	return fmt.Sprintf("VG%s-%04d", time.Now().Format("20060102"), seq)
}

func nextHandoverNo() string {
	var seq int
	db.QueryRow(`SELECT COALESCE(MAX(CAST(substring(handover_no from 12 for 4) AS int)),0)+1
		FROM shift_handovers WHERE handover_no LIKE 'HJ'||to_char(CURRENT_DATE,'YYYYMMDD')||'-%'`).Scan(&seq)
	return fmt.Sprintf("HJ%s-%04d", time.Now().Format("20060102"), seq)
}

// ---------- 双人入柜：站务发起 ----------
// POST /api/valuable/intakes
// 司机上交手机/钱包后，站务登记共同拍照、封袋号、柜号；入库后处于待会签状态，由安保会签确认。
func handleValuableIntakeCreate(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "security") {
		errJSON(w, 403, "仅站务可发起双人入柜（安保会签）")
		return
	}
	if u.Role == "security" {
		errJSON(w, 403, "入柜由站务发起，安保负责共同拍照与会签确认")
		return
	}
	var body struct {
		ItemID         int      `json:"item_id"`
		CabinetNo      string   `json:"cabinet_no"`
		SealNo         string   `json:"seal_no"`
		StationPhotos  []string `json:"station_photos"`
		Notes          string   `json:"notes"`
		// 站务也可直接补录物品信息（无待登记物品时）
		Category       string   `json:"category"`
		Description    string   `json:"description"`
		Features       string   `json:"features"`
		FoundLineID    *int     `json:"found_line_id"`
		FoundVehicleID *int     `json:"found_vehicle_id"`
		FoundStopID    *int     `json:"found_stop_id"`
		FoundAt        string   `json:"found_at"`
		HandedByRole   string   `json:"handed_by_role"`
		HandedByName   string   `json:"handed_by_name"`
		SecurityID     *int     `json:"security_id"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	body.CabinetNo = strings.TrimSpace(body.CabinetNo)
	body.SealNo = strings.TrimSpace(body.SealNo)
	if body.CabinetNo == "" || body.SealNo == "" {
		errJSON(w, 400, "柜号与封袋编号为必填项")
		return
	}
	if len(body.StationPhotos) == 0 {
		errJSON(w, 400, "入柜前需由站务与安保共同拍照（至少 1 张留档）")
		return
	}
	if body.ItemID == 0 {
		if body.Category == "" {
			errJSON(w, 400, "请选择物品类别")
			return
		}
		if !isValuableCategory(body.Category, "贵重") {
			errJSON(w, 400, "双人入柜仅适用于手机/钱包/贵重物品，普通物品请走常规登记入库")
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()

	itemID := body.ItemID
	if itemID == 0 {
		// 站务直接补录物品（不经司机端待登记）
		if strings.TrimSpace(body.Description) == "" {
			errJSON(w, 400, "物品描述为必填项")
			return
		}
		foundAt := parseDT(body.FoundAt)
		if foundAt == nil {
			t := time.Now()
			foundAt = &t
		}
		handedRole := body.HandedByRole
		if handedRole == "" {
			handedRole = "driver"
		}
		handedName := body.HandedByName
		if handedName == "" {
			handedName = u.Name
		}
		var fleetID any
		if body.FoundVehicleID != nil {
			var fid int
			if tx.QueryRow(`SELECT fleet_id FROM vehicles WHERE id=$1`, *body.FoundVehicleID).Scan(&fid) == nil {
				fleetID = fid
			}
		}
		if fleetID == nil && u.FleetID != nil {
			fleetID = *u.FleetID
		}
		if err := tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_stop_id,found_at,
			handed_by_role,handed_by_id,handed_by_name,fleet_id,status)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending_register') RETURNING id`,
			nextItemNo(), body.Category, body.Description, body.Features, body.FoundLineID, body.FoundVehicleID, body.FoundStopID, *foundAt,
			handedRole, u.ID, handedName, fleetID).Scan(&itemID); err != nil {
			errJSON(w, 400, "补录物品失败: "+err.Error())
			return
		}
	}

	// 读取物品并校验
	var category, desc, status string
	var fleetID sql.NullInt64
	if err := tx.QueryRow(`SELECT category, description, status, fleet_id FROM found_items WHERE id=$1`, itemID).
		Scan(&category, &desc, &status, &fleetID); err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	if !isValuableCategory(category, "贵重") {
		errJSON(w, 400, "仅手机/钱包/贵重物品需要双人入柜，普通物品请走常规登记入库")
		return
	}
	if status == "in_storage" {
		errJSON(w, 409, "该物品已完成入柜")
		return
	}
	if status != "pending_register" {
		errJSON(w, 409, "当前物品状态不允许双人入柜（"+status+"）")
		return
	}
	// 柜号唯一占用
	var occupied int
	tx.QueryRow(`SELECT count(*) FROM valuable_intakes WHERE cabinet_no=$1 AND status='completed'`, body.CabinetNo).Scan(&occupied)
	if occupied > 0 {
		errJSON(w, 400, "柜号 "+body.CabinetNo+" 已被其他贵重物品占用")
		return
	}

	// 贵重物品保管规则：90 天、保险柜、逾期移交公安
	rules := "贵重物品：站务与安保双人核验、共同拍照、封袋入柜，保险柜保管，联动安保确认；保管期 90 天，逾期移交公安机关。"
	retentionUntil := time.Now().AddDate(0, 0, 90)
	if _, err := tx.Exec(`UPDATE found_items SET storage_cabinet=$1, value_level='贵重', special_type='无',
		retention_days=90, retention_until=$2, storage_rules=$3, custodian_id=$4, cabinet_locked=TRUE,
		status='in_storage', registered_by=$5, updated_at=now() WHERE id=$6`,
		body.CabinetNo, retentionUntil, rules, u.ID, u.ID, itemID); err != nil {
		errJSON(w, 500, err.Error())
		return
	}

	var fid any
	if fleetID.Valid {
		fid = fleetID.Int64
	}
	var intakeID int
	if err := tx.QueryRow(`INSERT INTO valuable_intakes(intake_no,item_id,cabinet_no,seal_no,station_id,station_name,
		station_photos,notes,status,fleet_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending_countersign',$9) RETURNING id`,
		nextIntakeNo(), itemID, body.CabinetNo, body.SealNo, u.ID, u.Name, pq.Array(body.StationPhotos), body.Notes, fid).Scan(&intakeID); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(`INSERT INTO alarms(item_id,type,level,title) VALUES($1,'valuable','一般',$2)`,
		itemID, fmt.Sprintf("贵重物品双人入柜：%s，柜号 %s，封袋 %s，站务已发起、待安保会签确认", desc, body.CabinetNo, body.SealNo)); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, &itemID, "双人入柜(站务)", fmt.Sprintf("站务 %s 与安保共同拍照、封袋（封袋号 %s）、入柜 %s；待安保会签确认", u.Name, body.SealNo, body.CabinetNo))
	audit(u, "valuable_intake", intakeID, "intake_create", fmt.Sprintf("贵重物品入柜发起 物品#%d 柜=%s 封袋=%s", itemID, body.CabinetNo, body.SealNo))
	audit(u, "item", itemID, "register", fmt.Sprintf("贵重物品双人入柜 柜=%s 封袋=%s（待会签）", body.CabinetNo, body.SealNo))
	writeJSON(w, 200, map[string]any{"id": intakeID, "item_id": itemID, "cabinet_locked": true})
}

// GET /api/valuable/intakes?status=
func handleValuableIntakeList(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "security", "cs", "station_manager") {
		errJSON(w, 403, "无权查看入柜记录")
		return
	}
	where := "1=1"
	var args []any
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where = "vi.status=$1"
	}
	rows, err := db.Query(`SELECT vi.id, vi.intake_no, vi.item_id, i.item_no, i.description, i.category,
		vi.cabinet_no, vi.seal_no, vi.station_name, COALESCE(vi.security_name,''), vi.seal_status, vi.status,
		COALESCE(vi.fleet_id,0), vi.created_at, vi.completed_at
		FROM valuable_intakes vi JOIN found_items i ON i.id=vi.item_id
		WHERE `+where+` ORDER BY vi.id DESC LIMIT 200`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	var fleetID int
	for rows.Next() {
		var id, itemID int
		var intakeNo, itemNo, desc, category, cabinet, seal, station, security, sealStatus, status string
		var created time.Time
		var completed sql.NullTime
		rows.Scan(&id, &intakeNo, &itemID, &itemNo, &desc, &category, &cabinet, &seal, &station, &security, &sealStatus, &status, &fleetID, &created, &completed)
		m := map[string]any{"id": id, "intake_no": intakeNo, "item_id": itemID, "item_no": itemNo, "description": desc,
			"category": category, "cabinet_no": cabinet, "seal_no": seal, "station_name": station,
			"security_name": security, "seal_status": sealStatus, "status": status, "fleet_id": fleetID, "created_at": created}
		if completed.Valid {
			m["completed_at"] = completed.Time
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"intakes": list})
}

// GET /api/items/{id}/valuable —— 物品的双人入柜记录（脱敏后的照片按授权另行获取）
func handleItemValuable(w http.ResponseWriter, r *http.Request, u *User, id int) {
	writeJSON(w, 200, map[string]any{"intakes": queryItemValuable(u, id)})
}

func queryItemValuable(u *User, id int) []map[string]any {
	rows, err := db.Query(`SELECT vi.id, vi.intake_no, vi.cabinet_no, vi.seal_no, vi.station_id, vi.station_name,
		vi.security_id, COALESCE(vi.security_name,''), vi.seal_status, vi.status, vi.notes,
		array_length(vi.station_photos,1), array_length(vi.security_photos,1), vi.fleet_id, vi.created_at, vi.completed_at
		FROM valuable_intakes vi WHERE vi.item_id=$1 ORDER BY vi.id DESC`, id)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var intakeID int
		var stationID, securityID, fleetID sql.NullInt64
		var intakeNo, cabinet, seal, stationName, securityName, sealStatus, status, notes string
		var stationPhotoN, securityPhotoN sql.NullInt64
		var created time.Time
		var completed sql.NullTime
		rows.Scan(&intakeID, &intakeNo, &cabinet, &seal, &stationID, &stationName, &securityID, &securityName,
			&sealStatus, &status, &notes, &stationPhotoN, &securityPhotoN, &fleetID, &created, &completed)
		m := map[string]any{
			"id": intakeID, "intake_no": intakeNo, "cabinet_no": cabinet, "seal_no": seal,
			"station_id": stationID.Int64, "station_name": stationName,
			"security_id": securityID.Int64, "security_name": securityName,
			"seal_status": sealStatus, "status": status, "notes": notes,
			"station_photo_count": stationPhotoN.Int64, "security_photo_count": securityPhotoN.Int64,
			"created_at": created,
		}
		if completed.Valid {
			m["completed_at"] = completed.Time
		}
		// 完整照片：站务/安保等内部角色可直接核验；客服需持有效授权
		canSeePhotos := u.Role != "cs"
		if u.Role == "cs" {
			if ok, _, _ := hasValidGrant(id, u); ok {
				canSeePhotos = true
			}
		}
		if canSeePhotos {
			var sp, sep []string
			db.QueryRow(`SELECT station_photos, security_photos FROM valuable_intakes WHERE id=$1`, intakeID).Scan(pq.Array(&sp), pq.Array(&sep))
			m["station_photos"] = sp
			m["security_photos"] = sep
		}
		list = append(list, m)
	}
	return list
}

// POST /api/valuable/intakes/{id}/countersign —— 安保会签：核对封袋、补拍会签照片
func handleValuableCountersign(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r)
	var body struct {
		SecurityPhotos []string `json:"security_photos"`
		SealStatus     string   `json:"seal_status"`
		SecurityID     *int     `json:"security_id"`
		Notes          string   `json:"notes"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	securityID, securityName, err := resolveSecurityCountersigner(u, body.SecurityID)
	if err != nil {
		errJSON(w, 403, err.Error())
		return
	}
	if len(body.SecurityPhotos) == 0 {
		errJSON(w, 400, "安保会签需补拍封袋/入柜照片（至少 1 张）")
		return
	}
	sealStatus := body.SealStatus
	if sealStatus == "" {
		sealStatus = "intact"
	}
	if sealStatus != "intact" {
		errJSON(w, 400, "封袋破损不得入柜：请重新封袋后再发起入柜")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID, stationID int
	var stationName, cabinet, seal, desc string
	if err := tx.QueryRow(`SELECT vi.item_id, vi.station_id, vi.station_name, vi.cabinet_no, vi.seal_no, i.description
		FROM valuable_intakes vi JOIN found_items i ON i.id=vi.item_id
		WHERE vi.id=$1 AND vi.status='pending_countersign'`, id).
		Scan(&itemID, &stationID, &stationName, &cabinet, &seal, &desc); err != nil {
		errJSON(w, 404, "入柜记录不存在或已会签")
		return
	}
	if stationID == securityID {
		errJSON(w, 400, "站务与安保会签不能为同一人")
		return
	}
	if _, err := tx.Exec(`UPDATE valuable_intakes SET status='completed', security_id=$1, security_name=$2,
		security_photos=$3, seal_status=$4, notes=COALESCE(NULLIF($5,''),notes), completed_at=now() WHERE id=$6`,
		securityID, securityName, pq.Array(body.SecurityPhotos), sealStatus, body.Notes, id); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	// 会签完成后物品柜进入正常锁定保管（有明确责任人）
	if _, err := tx.Exec(`UPDATE found_items SET cabinet_locked=TRUE, claim_frozen=FALSE, lock_reason=NULL,
		custodian_id=$1, updated_at=now() WHERE id=$2`, stationID, itemID); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE alarms SET status='closed', handled_by=$1, handled_at=now(),
		handle_notes='安保会签确认：封袋完好、柜号一致，双人入柜完成'
		WHERE item_id=$2 AND type='valuable' AND status IN ('open','ack')`, securityID, itemID); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	secUser := &User{ID: securityID, Role: "security", Name: securityName}
	addEvent(secUser, nil, &itemID, "双人入柜(安保会签)", fmt.Sprintf("安保 %s 核对封袋 %s 完好、柜号 %s 一致，会签确认入柜，物品柜锁定保管", securityName, seal, cabinet))
	audit(secUser, "valuable_intake", id, "countersign", fmt.Sprintf("安保会签 物品#%d 柜=%s 封袋=%s 封袋完好", itemID, cabinet, seal))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------- 敏感信息授权（客服在乘客认领前仅见必要描述） ----------

// 客服是否对某物品有有效的敏感信息查看授权
func hasValidGrant(itemID int, u *User) (bool, int, time.Time) {
	var id int
	var validUntil time.Time
	err := db.QueryRow(`SELECT id, valid_until FROM sensitive_grants
		WHERE item_id=$1 AND requester_id=$2 AND status='approved' AND valid_until > now()
		ORDER BY id DESC LIMIT 1`, itemID, u.ID).Scan(&id, &validUntil)
	if err != nil {
		return false, 0, time.Time{}
	}
	return true, id, validUntil
}

// POST /api/items/{id}/sensitive/request —— 客服申请查看完整照片/证件
func handleSensitiveRequest(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r)
	if u.Role != "cs" {
		errJSON(w, 403, "仅客服需要申请敏感信息查看授权（站务/安保可直接核验查看）")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	readJSON(r, &body)
	if strings.TrimSpace(body.Reason) == "" {
		errJSON(w, 400, "请填写查看事由（如：与乘客核对认领信息）")
		return
	}
	if ok, _, _ := hasValidGrant(id, u); ok {
		errJSON(w, 409, "已有有效授权，无需重复申请")
		return
	}
	var desc string
	if err := db.QueryRow(`SELECT description FROM found_items WHERE id=$1`, id).Scan(&desc); err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	var gid int
	err := db.QueryRow(`INSERT INTO sensitive_grants(item_id,requester_id,requester_name,reason,valid_until)
		VALUES($1,$2,$3,$4,now()+interval '4 hours') RETURNING id`, id, u.ID, u.Name, body.Reason).Scan(&gid)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	audit(u, "sensitive_grant", gid, "request", fmt.Sprintf("申请查看物品#%d 完整照片/证件：%s", id, body.Reason))
	writeJSON(w, 200, map[string]any{"id": gid, "status": "pending"})
}

// GET /api/sensitive/requests?status=pending —— 站务/安保审批列表；客服看自己的申请
func handleSensitiveList(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "cs", "station", "security", "station_manager") {
		errJSON(w, 403, "无权查看")
		return
	}
	q := `SELECT g.id, g.item_id, i.item_no, i.description, g.requester_id, g.requester_name, COALESCE(g.reason,''),
		g.status, g.valid_until, COALESCE(g.approve_notes,''), COALESCE(au.name,''), g.created_at, g.approved_at
		FROM sensitive_grants g JOIN found_items i ON i.id=g.item_id
		LEFT JOIN users au ON au.id=g.approver_id `
	args := []any{}
	if u.Role == "cs" {
		q += `WHERE g.requester_id=$1 `
		args = append(args, u.ID)
	} else if s := r.URL.Query().Get("status"); s != "" {
		q += `WHERE g.status=$1 `
		args = append(args, s)
	}
	q += `ORDER BY g.id DESC LIMIT 100`
	rows, err := db.Query(q, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, itemID, requesterID sql.NullInt64
		var itemNo, desc, requester, reason, status, notes, approver string
		var validUntil, created time.Time
		var approved sql.NullTime
		rows.Scan(&id, &itemID, &itemNo, &desc, &requesterID, &requester, &reason, &status, &validUntil, &notes, &approver, &created, &approved)
		m := map[string]any{"id": id.Int64, "item_id": itemID.Int64, "item_no": itemNo, "description": desc,
			"requester_name": requester, "reason": reason, "status": status,
			"valid_until": validUntil, "approve_notes": notes, "approver": approver, "created_at": created}
		if approved.Valid {
			m["approved_at"] = approved.Time
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"grants": list})
}

// POST /api/sensitive/{id}/approve | /reject
func handleSensitiveApprove(w http.ResponseWriter, r *http.Request, u *User, approve bool) {
	if !requireRole(u, "station", "security", "station_manager") {
		errJSON(w, 403, "仅站务主管/站务/安保可审批敏感信息查看授权")
		return
	}
	id := pathID(r)
	var body struct {
		Notes string `json:"notes"`
		Hours int    `json:"hours"`
	}
	readJSON(r, &body)
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID, requesterID int
	var desc, requester string
	q := `UPDATE sensitive_grants SET status=$1, approver_id=$2, approved_at=now(), approve_notes=$3 WHERE id=$4 AND status='pending'
		RETURNING item_id, requester_id, requester_name`
	args := []any{}
	status := "rejected"
	if approve {
		status = "approved"
		hours := body.Hours
		if hours <= 0 {
			hours = 4
		}
		q = `UPDATE sensitive_grants SET status='approved', approver_id=$1, approved_at=now(), approve_notes=$2,
			valid_until=now()+($3 || ' hours')::interval WHERE id=$4 AND status='pending'
			RETURNING item_id, requester_id, requester_name`
		args = append(args, u.ID, body.Notes, fmt.Sprintf("%d", hours), id)
	} else {
		args = append(args, status, u.ID, body.Notes, id)
	}
	if err := tx.QueryRow(q, args...).Scan(&itemID, &requesterID, &requester); err != nil {
		errJSON(w, 409, "申请不存在或已处理")
		return
	}
	if err := tx.QueryRow(`SELECT description FROM found_items WHERE id=$1`, itemID).Scan(&desc); err != nil {
		desc = ""
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if approve {
		addEvent(u, nil, &itemID, "敏感信息授权", fmt.Sprintf("批准客服 %s 在有效期内查看完整照片/证件（%s）", requester, body.Notes))
		audit(u, "sensitive_grant", id, "approve", fmt.Sprintf("批准客服 %s 查看物品#%d 完整照片/证件", requester, itemID))
	} else {
		addEvent(u, nil, &itemID, "敏感信息授权", fmt.Sprintf("拒绝客服 %s 的查看申请：%s", requester, body.Notes))
		audit(u, "sensitive_grant", id, "reject", fmt.Sprintf("拒绝客服 %s 查看物品#%d：%s", requester, itemID, body.Notes))
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// GET /api/items/{id}/sensitive —— 授权后获取完整照片/证件（每次查看留痕）
func handleSensitiveView(w http.ResponseWriter, r *http.Request, u *User, id int) {
	if u.Role != "cs" {
		errJSON(w, 403, "该接口仅供授权客服获取敏感信息")
		return
	}
	ok, grantID, validUntil := hasValidGrant(id, u)
	if !ok {
		errJSON(w, 403, "尚无有效授权：乘客认领前完整照片/证件需授权查看")
		return
	}
	var photos pq.StringArray
	var stationPhotos, securityPhotos pq.StringArray
	var category string
	err := db.QueryRow(`SELECT i.photos, i.category,
		COALESCE((SELECT station_photos FROM valuable_intakes WHERE item_id=i.id ORDER BY id DESC LIMIT 1),'{}'),
		COALESCE((SELECT security_photos FROM valuable_intakes WHERE item_id=i.id ORDER BY id DESC LIMIT 1),'{}')
		FROM found_items i WHERE i.id=$1`, id).Scan(&photos, &category, &stationPhotos, &securityPhotos)
	if err != nil {
		errJSON(w, 404, "物品不存在")
		return
	}
	// 关联招领单的乘客证件号（认领核对需要时可见，仍逐次留痕）
	var idCards []string
	crows, _ := db.Query(`SELECT DISTINCT COALESCE(r.passenger_id_card,'') FROM lost_reports r
		WHERE r.id IN (SELECT report_id FROM claims WHERE item_id=$1 AND report_id IS NOT NULL)`, id)
	for crows.Next() {
		var s string
		crows.Scan(&s)
		if s != "" {
			idCards = append(idCards, s)
		}
	}
	crows.Close()
	db.Exec(`INSERT INTO sensitive_access_logs(grant_id,item_id,viewer_id,viewer_name,scope,purpose)
		VALUES($1,$2,$3,$4,'full','客服授权查看完整照片/证件')`, grantID, id, u.ID, u.Name)
	audit(u, "item", id, "view_sensitive", fmt.Sprintf("客服凭授权#%d 查看完整照片/证件（有效期至 %s）", grantID, validUntil.Format("01-02 15:04")))
	writeJSON(w, 200, map[string]any{
		"photos":           []string(photos),
		"station_photos":   []string(stationPhotos),
		"security_photos":  []string(securityPhotos),
		"report_id_cards":  idCards,
		"valid_until":      validUntil,
	})
}

// ---------- 站务换班交接 ----------

// GET /api/handovers/my-vault —— 当前站务负责的在柜贵重物品（交接清单来源）
func handleMyVault(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务查看本人保管柜清单")
		return
	}
	rows, err := db.Query(`SELECT i.id, i.item_no, i.category, i.description, i.storage_cabinet,
		i.cabinet_locked, i.claim_frozen, COALESCE(v.seal_no,''), COALESCE(v.seal_status,'intact')
		FROM found_items i
		JOIN valuable_intakes v ON v.item_id=i.id AND v.status='completed'
		WHERE i.custodian_id=$1 AND i.status='in_storage' AND i.claim_frozen=FALSE ORDER BY i.storage_cabinet`, u.ID)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int
		var itemNo, category, desc, cabinet, sealNo, sealStatus string
		var locked, frozen bool
		rows.Scan(&id, &itemNo, &category, &desc, &cabinet, &locked, &frozen, &sealNo, &sealStatus)
		list = append(list, map[string]any{
			"id": id, "item_no": itemNo, "category": category, "description": desc,
			"cabinet_no": cabinet, "seal_no": sealNo, "seal_status": sealStatus,
			"locked": locked, "frozen": frozen,
		})
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

// POST /api/handovers —— 交班站务发起交接（选择接班人）
func handleHandoverCreate(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅站务可发起换班交接")
		return
	}
	var body struct {
		ToStationID int `json:"to_station_id"`
	}
	if err := readJSON(r, &body); err != nil || body.ToStationID == 0 {
		errJSON(w, 400, "请选择接班站务")
		return
	}
	if body.ToStationID == u.ID {
		errJSON(w, 400, "接班人不能是本人")
		return
	}
	var trole, tname string
	var tfleet sql.NullInt64
	if err := db.QueryRow(`SELECT role, name, fleet_id FROM users WHERE id=$1`, body.ToStationID).Scan(&trole, &tname, &tfleet); err != nil {
		errJSON(w, 404, "接班站务不存在")
		return
	}
	if trole != "station" {
		errJSON(w, 400, "接班人必须是站务角色")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	// 本人负责且在库的贵重物品
	rows, err := tx.Query(`SELECT i.id, i.storage_cabinet, v.seal_no
		FROM found_items i
		JOIN valuable_intakes v ON v.item_id=i.id AND v.status='completed'
		WHERE i.custodian_id=$1 AND i.status='in_storage' AND i.claim_frozen=FALSE`, u.ID)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	type hi struct {
		itemID  int
		cabinet string
		seal    string
	}
	var items []hi
	for rows.Next() {
		var x hi
		rows.Scan(&x.itemID, &x.cabinet, &x.seal)
		items = append(items, x)
	}
	rows.Close()
	if len(items) == 0 {
		errJSON(w, 400, "您名下没有在柜贵重物品需要交接")
		return
	}
	var fid any
	if u.FleetID != nil {
		fid = *u.FleetID
	}
	var hid int
	if err := tx.QueryRow(`INSERT INTO shift_handovers(handover_no,shift_date,from_station_id,to_station_id,fleet_id,status)
		VALUES($1,CURRENT_DATE,$2,$3,$4,'pending') RETURNING id`, nextHandoverNo(), u.ID, body.ToStationID, fid).Scan(&hid); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	for _, x := range items {
		tx.Exec(`INSERT INTO handover_items(handover_id,item_id,cabinet_no,seal_no) VALUES($1,$2,$3,$4)`,
			hid, x.itemID, x.cabinet, x.seal)
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	addEvent(u, nil, nil, "发起换班交接", fmt.Sprintf("交班站务 %s 向接班人 %s 交接 %d 件在柜贵重物品，待逐项核对柜号与封袋状态", u.Name, tname, len(items)))
	audit(u, "handover", hid, "create", fmt.Sprintf("发起交接 → %s，贵重物品 %d 件", tname, len(items)))
	writeJSON(w, 200, map[string]any{"id": hid, "count": len(items)})
}

// GET /api/handovers?scope=todo|created|all
func handleHandoverList(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "station_manager", "security") {
		errJSON(w, 403, "无权查看交接")
		return
	}
	where := "1=1"
	args := []any{}
	switch r.URL.Query().Get("scope") {
	case "todo":
		where = "h.to_station_id=$1 AND h.status IN ('pending','abnormal')"
		args = append(args, u.ID)
	case "created":
		where = "h.from_station_id=$1"
		args = append(args, u.ID)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where += fmt.Sprintf(" AND h.status=$%d", len(args))
	}
	rows, err := db.Query(`SELECT h.id, h.handover_no, h.shift_date, h.from_station_id, h.to_station_id,
		u1.name, u2.name, h.status, COALESCE(h.check_notes,''), h.abnormal_count, h.created_at, h.checked_at,
		COALESCE(h.resolve_notes,''), COALESCE(u3.name,'')
		FROM shift_handovers h
		JOIN users u1 ON u1.id=h.from_station_id JOIN users u2 ON u2.id=h.to_station_id
		LEFT JOIN users u3 ON u3.id=h.reviewed_by
		WHERE `+where+` ORDER BY h.id DESC LIMIT 100`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, fromID, toID, abnormal int
		var no string
		var shiftDate time.Time
		var fromName, toName, status, notes, resolveNotes, reviewer string
		var created time.Time
		var checked sql.NullTime
		rows.Scan(&id, &no, &shiftDate, &fromID, &toID, &fromName, &toName, &status, &notes, &abnormal, &created, &checked, &resolveNotes, &reviewer)
		m := map[string]any{"id": id, "handover_no": no, "shift_date": shiftDate.Format("2006-01-02"),
			"from_station_id": fromID, "from_station": fromName, "to_station_id": toID, "to_station": toName,
			"status": status, "check_notes": notes, "abnormal_count": abnormal, "created_at": created,
			"resolve_notes": resolveNotes, "reviewed_by_name": reviewer}
		if checked.Valid {
			m["checked_at"] = checked.Time
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"handovers": list})
}

// GET /api/handovers/{id} —— 交接单详情（含逐项核对表）
func handleHandoverGet(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "station_manager", "security") {
		errJSON(w, 403, "无权查看交接")
		return
	}
	id := pathID(r)
	var fromID, toID, abnormal int
	var no string
	var shiftDate time.Time
	var fromName, toName, status, notes string
	var created time.Time
	var checked sql.NullTime
	err := db.QueryRow(`SELECT h.handover_no, h.shift_date, h.from_station_id, h.to_station_id, u1.name, u2.name,
		h.status, COALESCE(h.check_notes,''), h.abnormal_count, h.created_at, h.checked_at
		FROM shift_handovers h JOIN users u1 ON u1.id=h.from_station_id JOIN users u2 ON u2.id=h.to_station_id
		WHERE h.id=$1`, id).Scan(&no, &shiftDate, &fromID, &toID, &fromName, &toName, &status, &notes, &abnormal, &created, &checked)
	if err != nil {
		errJSON(w, 404, "交接单不存在")
		return
	}
	rows, _ := db.Query(`SELECT hi.id, hi.item_id, i.item_no, i.description, hi.cabinet_no, COALESCE(hi.seal_no,''),
		hi.check_result, COALESCE(hi.notes,'')
		FROM handover_items hi JOIN found_items i ON i.id=hi.item_id WHERE hi.handover_id=$1 ORDER BY hi.id`, id)
	items := []map[string]any{}
	for rows.Next() {
		var hiID, itemID int
		var itemNo, desc, cabinet, seal, result, inotes string
		rows.Scan(&hiID, &itemID, &itemNo, &desc, &cabinet, &seal, &result, &inotes)
		items = append(items, map[string]any{"id": hiID, "item_id": itemID, "item_no": itemNo, "description": desc,
			"cabinet_no": cabinet, "seal_no": seal, "check_result": result, "notes": inotes})
	}
	rows.Close()
	out := map[string]any{"id": id, "handover_no": no, "shift_date": shiftDate.Format("2006-01-02"),
		"from_station_id": fromID, "from_station": fromName, "to_station_id": toID, "to_station": toName,
		"status": status, "check_notes": notes, "abnormal_count": abnormal, "created_at": created, "items": items}
	if checked.Valid {
		out["checked_at"] = checked.Time
	}
	writeJSON(w, 200, out)
}

// POST /api/handovers/{id}/check —— 接班人逐项核对
// body: { results: [{item_id, result: ok/mismatch/damaged/missing, notes}] }
func handleHandoverCheck(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station") {
		errJSON(w, 403, "仅接班站务可核对交接")
		return
	}
	id := pathID(r)
	var body struct {
		Results []struct {
			ItemID int    `json:"item_id"`
			Result string `json:"result"`
			Notes  string `json:"notes"`
		} `json:"results"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Results) == 0 {
		errJSON(w, 400, "请提交逐项核对结果")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var toID, fromID int
	var status string
	if err := tx.QueryRow(`SELECT to_station_id, from_station_id, status FROM shift_handovers WHERE id=$1`, id).
		Scan(&toID, &fromID, &status); err != nil {
		errJSON(w, 404, "交接单不存在")
		return
	}
	if toID != u.ID {
		errJSON(w, 403, "仅指定的接班人可核对本交接单")
		return
	}
	if status != "pending" {
		errJSON(w, 409, "该交接单已核对完成")
		return
	}
	valid := map[string]bool{"ok": true, "mismatch": true, "damaged": true, "missing": true}
	abnormal := []string{}
	checked := map[int]bool{}
	for _, x := range body.Results {
		if !valid[x.Result] {
			errJSON(w, 400, "核对结果取值非法")
			return
		}
		checked[x.ItemID] = true
		if _, err := tx.Exec(`UPDATE handover_items SET check_result=$1, notes=$2 WHERE handover_id=$3 AND item_id=$4`,
			x.Result, x.Notes, id, x.ItemID); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		if x.Result != "ok" {
			abnormal = append(abnormal, fmt.Sprintf("物品#%d:%s", x.ItemID, map[string]string{
				"mismatch": "柜号不符", "damaged": "封袋破损", "missing": "物品缺失",
			}[x.Result]))
		}
	}
	// 所有明细都必须核对
	var total, done int
	tx.QueryRow(`SELECT count(*), count(*) FILTER (WHERE check_result<>'pending') FROM handover_items WHERE handover_id=$1`, id).Scan(&total, &done)
	if done < total {
		errJSON(w, 400, fmt.Sprintf("仍有 %d 件未核对（贵重物品逐项核对，不得遗漏）", total-done))
		return
	}

	if len(abnormal) == 0 {
		// 正常交接：责任人转给接班人，物品柜保持锁定保管
		tx.Exec(`UPDATE shift_handovers SET status='normal', check_notes='逐项核对柜号与封袋状态均正常', checked_at=now() WHERE id=$1`, id)
		tx.Exec(`UPDATE found_items SET custodian_id=$1, cabinet_locked=TRUE, claim_frozen=FALSE, lock_reason=NULL, updated_at=now()
			WHERE id IN (SELECT item_id FROM handover_items WHERE handover_id=$2)`, toID, id)
		if err := tx.Commit(); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		addEvent(u, nil, nil, "换班交接正常", fmt.Sprintf("接班人 %s 逐项核对 %d 件贵重物品：柜号一致、封袋完好，责任已交接；物品柜保持锁定", u.Name, total))
		audit(u, "handover", id, "check_normal", fmt.Sprintf("交接核对正常，共 %d 件，责任人→%s", total, u.Name))
		writeJSON(w, 200, map[string]any{"status": "normal", "abnormal_count": 0})
		return
	}

	// 异常交接：物品柜保持锁定、冻结认领，生成主管复核任务
	notes := strings.Join(abnormal, "；")
	tx.Exec(`UPDATE shift_handovers SET status='abnormal', abnormal_count=$1, check_notes=$2, checked_at=now() WHERE id=$3`,
		len(abnormal), "交接核对异常："+notes, id)
	// 异常物品：锁定 + 冻结，责任人暂挂交班人（不能无人负责）
	tx.Exec(`UPDATE found_items SET cabinet_locked=TRUE, claim_frozen=TRUE,
		lock_reason=$1, custodian_id=$2, updated_at=now()
		WHERE id IN (SELECT item_id FROM handover_items WHERE handover_id=$3 AND check_result<>'ok')`,
		"换班交接异常，等待站务主管复核："+notes, fromID, id)
	// 正常项责任仍转移
	tx.Exec(`UPDATE found_items SET custodian_id=$1, cabinet_locked=TRUE, updated_at=now()
		WHERE id IN (SELECT item_id FROM handover_items WHERE handover_id=$2 AND check_result='ok')`, toID, id)
	// 每件异常物品一个复核任务（先完整读取并关闭 rows，再在事务上执行写入）
	var itemID int
	var itemNo, desc, cabinet, result string
	arows, _ := tx.Query(`SELECT hi.item_id, i.item_no, i.description, hi.cabinet_no, hi.check_result
		FROM handover_items hi JOIN found_items i ON i.id=hi.item_id
		WHERE hi.handover_id=$1 AND hi.check_result<>'ok'`, id)
	type abnormalItem struct {
		itemID      int
		title       string
		resultLabel string
	}
	var abnormalItems []abnormalItem
	for arows.Next() {
		arows.Scan(&itemID, &itemNo, &desc, &cabinet, &result)
		label := map[string]string{"mismatch": "柜号不符", "damaged": "封袋破损", "missing": "物品缺失"}[result]
		abnormalItems = append(abnormalItems, abnormalItem{
			itemID:      itemID,
			title:       fmt.Sprintf("换班交接异常复核：%s（%s）柜 %s · %s", itemNo, desc, cabinet, label),
			resultLabel: label,
		})
	}
	arows.Close()
	for _, ab := range abnormalItems {
		tx.QueryRow(`INSERT INTO review_tasks(item_id,handover_id,type,title,detail,created_by)
			VALUES($1,$2,'handover_abnormal',$3,$4,$5)`,
			ab.itemID, id, ab.title, fmt.Sprintf("交接单 #%d 核对结果：%s", id, ab.resultLabel), fromID)
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	for _, ab := range abnormalItems {
		iid := ab.itemID
		addEvent(u, nil, &iid, "交接异常", "接班人核对异常（"+ab.resultLabel+"）：物品柜保持锁定、认领已冻结，等待站务主管复核")
		audit(u, "item", iid, "handover_abnormal", "交接异常：柜锁定+认领冻结，待主管复核（"+ab.resultLabel+"）")
	}
	audit(u, "handover", id, "check_abnormal", notes)
	writeJSON(w, 200, map[string]any{"status": "abnormal", "abnormal_count": len(abnormal), "detail": notes})
}

// ---------- 站务主管复核 ----------

// GET /api/reviews?status=pending
func handleReviewList(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station_manager", "security") {
		errJSON(w, 403, "仅站务主管可查看复核任务")
		return
	}
	where := "1=1"
	args := []any{}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where = "t.status=$1"
	}
	rows, err := db.Query(`SELECT t.id, COALESCE(t.item_id,0), COALESCE(i.item_no,''), t.handover_id, t.type, t.title,
		COALESCE(t.detail,''), t.status, COALESCE(t.resolution,''), COALESCE(cu.name,''), COALESCE(ru.name,''),
		t.created_at, t.resolved_at
		FROM review_tasks t LEFT JOIN found_items i ON i.id=t.item_id
		LEFT JOIN users cu ON cu.id=t.created_by LEFT JOIN users ru ON ru.id=t.resolved_by
		WHERE `+where+` ORDER BY CASE t.status WHEN 'pending' THEN 0 ELSE 1 END, t.id DESC LIMIT 100`, args...)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, itemID, handoverID sql.NullInt64
		var itemNo, typ, title, detail, status, resolution, creator, resolver string
		var created time.Time
		var resolved sql.NullTime
		rows.Scan(&id, &itemID, &itemNo, &handoverID, &typ, &title, &detail, &status, &resolution, &creator, &resolver, &created, &resolved)
		m := map[string]any{"id": id.Int64, "item_id": itemID.Int64, "item_no": itemNo, "handover_id": handoverID.Int64,
			"type": typ, "title": title, "detail": detail, "status": status, "resolution": resolution,
			"created_by": creator, "resolved_by": resolver, "created_at": created}
		if resolved.Valid {
			m["resolved_at"] = resolved.Time
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"reviews": list})
}

// POST /api/reviews/{id}/resolve —— 主管复核：确认物品在位并恢复责任/解冻；或确认丢失维持冻结上报
// body: { action: restore(找回/核对无误，解除锁定冻结) | escalate(确认异常，维持冻结并上报), resolution, new_custodian_id }
func handleReviewResolve(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station_manager") {
		errJSON(w, 403, "仅站务主管可执行复核")
		return
	}
	id := pathID(r)
	var body struct {
		Action          string `json:"action"`
		Resolution      string `json:"resolution"`
		NewCustodianID  *int   `json:"new_custodian_id"`
	}
	if err := readJSON(r, &body); err != nil {
		errJSON(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(body.Resolution) == "" {
		errJSON(w, 400, "请填写复核结论")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var itemID, handoverID sql.NullInt64
	var title string
	if err := tx.QueryRow(`UPDATE review_tasks SET status='resolved', resolution=$1, resolved_by=$2, resolved_at=now()
		WHERE id=$3 AND status='pending' RETURNING item_id, handover_id, title`,
		body.Resolution, u.ID, id).Scan(&itemID, &handoverID, &title); err != nil {
		errJSON(w, 409, "复核任务不存在或已处理")
		return
	}
	if body.Action == "restore" {
		custodian := u.ID
		if body.NewCustodianID != nil && *body.NewCustodianID > 0 {
			custodian = *body.NewCustodianID
		} else if handoverID.Valid {
			tx.QueryRow(`SELECT to_station_id FROM shift_handovers WHERE id=$1`, handoverID.Int64).Scan(&custodian)
		}
		if itemID.Valid {
			tx.Exec(`UPDATE found_items SET cabinet_locked=TRUE, claim_frozen=FALSE, lock_reason=NULL,
				custodian_id=$1, updated_at=now() WHERE id=$2`, custodian, itemID.Int64)
		}
		if handoverID.Valid {
			// 若该交接单下所有异常任务均已复核，交接单闭环
			var pending int
			tx.QueryRow(`SELECT count(*) FROM review_tasks WHERE handover_id=$1 AND status='pending'`, handoverID.Int64).Scan(&pending)
			if pending == 0 {
				tx.Exec(`UPDATE shift_handovers SET status='reviewed', reviewed_by=$1, reviewed_at=now(),
					resolve_notes=$2 WHERE id=$3`, u.ID, body.Resolution, handoverID.Int64)
			}
		}
	} else {
		// 确认异常/升级上报：维持柜锁定与认领冻结
		if itemID.Valid {
			tx.Exec(`UPDATE found_items SET cabinet_locked=TRUE, claim_frozen=TRUE, lock_reason=$1, updated_at=now() WHERE id=$2`,
				"主管复核确认异常，维持冻结："+body.Resolution, itemID.Int64)
			tx.Exec(`INSERT INTO alarms(item_id,type,level,title,handle_notes) VALUES($1,'valuable','紧急',$2,$3)`,
				itemID.Int64, "贵重物品交接异常经主管复核确认："+title, body.Resolution)
		}
		if handoverID.Valid {
			tx.Exec(`UPDATE shift_handovers SET resolve_notes=$1 WHERE id=$2`, body.Resolution, handoverID.Int64)
		}
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if itemID.Valid {
		iid := int(itemID.Int64)
		if body.Action == "restore" {
			addEvent(u, nil, &iid, "主管复核完成", "站务主管复核完成："+body.Resolution+"；物品柜解除异常锁定状态（仍正常上锁保管），认领解冻")
			audit(u, "review_task", id, "restore", "复核解除异常：物品#"+fmt.Sprint(iid)+" 认领解冻；"+body.Resolution)
		} else {
			addEvent(u, nil, &iid, "主管复核-维持冻结", "复核确认异常并升级："+body.Resolution+"；物品柜保持锁定，认领继续冻结，已紧急联动安保")
			audit(u, "review_task", id, "escalate", "复核维持冻结：物品#"+fmt.Sprint(iid)+"；"+body.Resolution)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
