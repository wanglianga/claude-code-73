package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schemaSQL string

func mustConnect() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://lostfound:lostfound@db:5432/lostfound?sslmode=disable"
	}
	var d *sql.DB
	var err error
	for i := 0; i < 30; i++ {
		d, err = sql.Open("postgres", dsn)
		if err == nil {
			err = d.Ping()
		}
		if err == nil {
			log.Println("connected to postgres")
			return d
		}
		log.Printf("waiting for postgres: %v", err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("cannot connect to postgres: %v", err)
	return nil
}

func migrate() {
	if _, err := db.Exec(schemaSQL); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("schema migrated")
}

func hash(pw string) string {
	b, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b)
}

func seed() {
	var n int
	db.QueryRow(`SELECT count(*) FROM users`).Scan(&n)
	if n > 0 {
		return
	}
	log.Println("seeding demo data...")
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	defer tx.Rollback()

	// 车队
	var fleet1, fleet2 int
	tx.QueryRow(`INSERT INTO fleets(name) VALUES('一分公司') RETURNING id`).Scan(&fleet1)
	tx.QueryRow(`INSERT INTO fleets(name) VALUES('二分公司') RETURNING id`).Scan(&fleet2)

	// 线路
	var line1, line2, line10 int
	tx.QueryRow(`INSERT INTO lines(code,name,fleet_id) VALUES('1','1路',$1) RETURNING id`, fleet1).Scan(&line1)
	tx.QueryRow(`INSERT INTO lines(code,name,fleet_id) VALUES('2','2路',$1) RETURNING id`, fleet1).Scan(&line2)
	tx.QueryRow(`INSERT INTO lines(code,name,fleet_id) VALUES('10','10路',$1) RETURNING id`, fleet2).Scan(&line10)

	// 站点
	stops := map[int][]string{
		line1:  {"火车站", "人民广场", "新华书店", "人民医院", "体育馆", "大学城"},
		line2:  {"火车站", "市政府", "文化中心", "滨江公园", "汽车东站"},
		line10: {"大学城", "科技园", "软件园", "高铁南站"},
	}
	stopIDs := map[string]int{} // "lineID:站名" -> id
	for lineID, names := range stops {
		for i, name := range names {
			var id int
			tx.QueryRow(`INSERT INTO stops(line_id,name,seq) VALUES($1,$2,$3) RETURNING id`, lineID, name, i+1).Scan(&id)
			stopIDs[fmt.Sprintf("%d:%s", lineID, name)] = id
		}
	}

	// 车辆
	type veh struct {
		id    int
		plate string
		line  int
		fleet int
	}
	vehs := []veh{}
	for _, v := range []struct {
		plate string
		line  int
		fleet int
	}{
		{"京A·D1001", line1, fleet1}, {"京A·D1002", line1, fleet1},
		{"京A·D2001", line2, fleet1},
		{"京B·D1001", line10, fleet2}, {"京B·D1002", line10, fleet2},
	} {
		var id int
		tx.QueryRow(`INSERT INTO vehicles(plate_no,line_id,fleet_id) VALUES($1,$2,$3) RETURNING id`, v.plate, v.line, v.fleet).Scan(&id)
		vehs = append(vehs, veh{id, v.plate, v.line, v.fleet})
	}

	// 用户（密码均为 123456）
	pw := hash("123456")
	type usr struct {
		username, role, name, phone string
		fleet                       int
	}
	users := []usr{
		{"admin", "admin", "系统管理员", "13800000000", 0},
		{"passenger", "passenger", "李明", "13911112222", 0},
		{"passenger2", "passenger", "王奶奶", "13933334444", 0},
		{"cs", "cs", "王晓", "13855550001", 0},
		{"driver", "driver", "张建国", "13855550002", fleet1},
		{"driver2", "driver", "刘洋", "13855550003", fleet2},
		{"station", "station", "李婷", "13855550004", fleet1},
		{"station2", "station", "周杰", "13855550005", fleet2},
		{"dispatcher", "dispatcher", "赵敏", "13855550006", 0},
		{"security", "security", "陈刚", "13855550007", 0},
	}
	uid := map[string]int{}
	for _, u := range users {
		var id int
		var fleet any
		if u.fleet == 0 {
			fleet = nil
		} else {
			fleet = u.fleet
		}
		tx.QueryRow(`INSERT INTO users(username,password_hash,role,name,phone,fleet_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
			u.username, pw, u.role, u.name, u.phone, fleet).Scan(&id)
		uid[u.username] = id
	}

	// 司机排班（今天）
	today := time.Now().Format("2006-01-02")
	tx.Exec(`INSERT INTO driver_shifts(driver_id,vehicle_id,shift_date,start_time,end_time) VALUES($1,$2,$3,'07:00','15:00')`, uid["driver"], vehs[0].id, today)
	tx.Exec(`INSERT INTO driver_shifts(driver_id,vehicle_id,shift_date,start_time,end_time) VALUES($1,$2,$3,'07:00','15:00')`, uid["driver2"], vehs[3].id, today)

	// GPS 轨迹 + 刷卡记录（今天 07:00-10:30，每 8 分钟一条）
	rnd := rand.New(rand.NewSource(42))
	now := time.Now()
	base := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, time.Local)
	for _, v := range vehs {
		names := stops[v.line]
		for i := 0; i < 40; i++ {
			ts := base.Add(time.Duration(i*8) * time.Minute)
			stopName := names[(i/2)%len(names)]
			sid := stopIDs[fmt.Sprintf("%d:%s", v.line, stopName)]
			lat := 31.20 + rnd.Float64()*0.1
			lng := 121.40 + rnd.Float64()*0.1
			tx.Exec(`INSERT INTO gps_pings(vehicle_id,ts,stop_id,lat,lng) VALUES($1,$2,$3,$4,$5)`, v.id, ts, sid, lat, lng)
			if i%3 == 0 {
				for k := 0; k < rnd.Intn(4)+1; k++ {
					card := fmt.Sprintf("CARD%06d", rnd.Intn(900000))
					tx.Exec(`INSERT INTO card_swipes(vehicle_id,card_no,ts,stop_id) VALUES($1,$2,$3,$4)`, v.id, card, ts.Add(time.Duration(k)*time.Minute), sid)
				}
			}
		}
	}

	// ---- 演示招领单：李明在 1 路遗失手机（已进入调度排查）----
	ts := func(h, m int) time.Time { return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.Local) }
	var reportID int
	err = tx.QueryRow(`INSERT INTO lost_reports(report_no,passenger_id,category,description,features,line_id,vehicle_id,vehicle_unknown,
		board_stop_id,alight_stop_id,ride_start,ride_end,seat_position,contact_name,contact_phone,passenger_id_card,status,next_role,assigned_cs_id)
		VALUES($1,$2,'手机','黑色 iPhone 14，手机壳有卡通贴纸，屏幕右上角有裂痕','卡通贴纸/屏幕裂痕/黑色',$3,NULL,TRUE,$4,$5,$6,$7,'后排靠窗','李明','13911112222','310104199505123214','searching','dispatcher',$8) RETURNING id`,
		"LP"+now.Format("20060102")+"-0001", uid["passenger"], line1,
		stopIDs[fmt.Sprintf("%d:火车站", line1)], stopIDs[fmt.Sprintf("%d:人民医院", line1)],
		ts(7, 50), ts(8, 20), uid["cs"]).Scan(&reportID)
	if err != nil {
		log.Fatalf("seed report: %v", err)
	}
	seedEvent(tx, &reportID, nil, uid["passenger"], "passenger", "李明", "乘客申报", "申报遗失：黑色 iPhone 14（1路，火车站→人民医院）")
	seedEvent(tx, &reportID, nil, uid["cs"], "cs", "王晓", "客服受理", "已受理并登记联系方式，转调度排查")
	seedEvent(tx, &reportID, nil, uid["dispatcher"], "dispatcher", "赵敏", "调度排查", "乘客记不清车牌；按 07:50 火车站上车、08:20 人民医院下车，结合 GPS 与刷卡记录，候选车辆：京A·D1001、京A·D1002")
	seedAudit(tx, uid["passenger"], "passenger", "李明", "report", reportID, "create", "创建招领单")
	seedAudit(tx, uid["cs"], "cs", "王晓", "report", reportID, "accept", "客服受理，转调度排查")
	seedAudit(tx, uid["dispatcher"], "dispatcher", "赵敏", "report", reportID, "dispatch_note", "GPS/刷卡排查，候选车辆：京A·D1001、京A·D1002")

	// ---- 演示物品 ----
	retention := func(days int) time.Time { return now.AddDate(0, 0, days) }
	// 1. 黑色 iPhone（贵重，可匹配上面的招领单）
	var itemPhone int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
		VALUES($1,'手机','黑色 iPhone 手机一部，卡通贴纸手机壳','卡通贴纸/屏幕有裂痕',$2,$3,$4,'driver',$5,'张建国',
		'A-01','贵重','无',90,$6,'贵重物品：双人核验入库，保险柜保管，联动安保确认；保管期 90 天，逾期移交公安机关。',$7,'in_storage',$8) RETURNING id`,
		"WP"+now.Format("20060102")+"-0001", line1, vehs[0].id, ts(9, 10), uid["driver"], retention(90), fleet1, uid["station"]).Scan(&itemPhone)
	seedEvent(tx, nil, &itemPhone, uid["driver"], "driver", "张建国", "司机上交", "1路 京A·D1001 车厢后排捡到黑色手机一部")
	seedEvent(tx, nil, &itemPhone, uid["station"], "station", "李婷", "站务登记入库", "存放柜 A-01，贵重，保管期 90 天")
	tx.Exec(`INSERT INTO alarms(item_id,type,level,title) VALUES($1,'valuable','一般','贵重物品入库：黑色 iPhone 手机（柜 A-01），请安保确认保管措施')`, itemPhone)
	seedAudit(tx, uid["driver"], "driver", "张建国", "item", itemPhone, "handin", "司机上交手机")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemPhone, "register", "登记入库 柜=A-01 贵重=贵重")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemPhone, "alarm", "触发贵重物品报警联动")

	// 2. 银行卡（特殊保管：提醒挂失，15 天，到期剪角销毁）
	var itemBank int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
		VALUES($1,'银行卡','工商银行借记卡一张（尾号 6688）','卡面有磨损',$2,$3,$4,'cleaner',NULL,'保洁-王阿姨',
		'B-03','普通','银行卡',15,$5,'银行卡：立即电话提醒失主挂失；保管期 15 天；逾期剪角销毁并留存影像。',$6,'in_storage',$7) RETURNING id`,
		"WP"+now.Format("20060102")+"-0002", line2, vehs[2].id, ts(8, 40), retention(15), fleet1, uid["station"]).Scan(&itemBank)
	seedEvent(tx, nil, &itemBank, uid["station"], "station", "李婷", "站务登记入库", "银行卡入柜 B-03，已触发挂失提醒告知流程")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemBank, "register", "登记入库 柜=B-03 特殊=银行卡")

	// 3. 药品（冷藏，7 天，到期销毁）
	var itemMed int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
		VALUES($1,'药品','胰岛素注射液一盒（需冷藏）',$2,$3,$4,'driver',$5,'刘洋',
		'冷藏柜-C1','普通','药品',7,$6,'药品：冷藏柜 2-8℃ 保管；保管期 7 天；逾期按医疗废弃物规范销毁（双人执行）。',$7,'in_storage',$8) RETURNING id`,
		"WP"+now.Format("20060102")+"-0003", line10, vehs[3].id, ts(8, 5), uid["driver2"], retention(7), fleet2, uid["station2"]).Scan(&itemMed)
	seedEvent(tx, nil, &itemMed, uid["station2"], "station", "周杰", "站务登记入库", "药品入冷藏柜 C1，保管期 7 天")
	seedAudit(tx, uid["station2"], "station", "周杰", "item", itemMed, "register", "登记入库 柜=冷藏柜-C1 特殊=药品")

	// 4. 儿童证件（优先联系监护人，90 天）
	var itemKid int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
		VALUES($1,'儿童物品','小学生学生证一张（阳光小学 三年级 王小明）','蓝色卡套',$2,$3,$4,'driver',$5,'张建国',
		'A-02','普通','儿童证件',90,$6,'儿童证件：优先电话联系监护人并站内公告；保管期 90 天；逾期移交学校或公安机关。',$7,'in_storage',$8) RETURNING id`,
		"WP"+now.Format("20060102")+"-0004", line1, vehs[0].id, ts(9, 30), uid["driver"], retention(90), fleet1, uid["station"]).Scan(&itemKid)
	seedEvent(tx, nil, &itemKid, uid["station"], "station", "李婷", "站务登记入库", "儿童证件入柜 A-02，已按流程联系监护人")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemKid, "register", "登记入库 柜=A-02 特殊=儿童证件")

	// 5. 危险品（紧急报警，安保暂存，3 天内移交公安）
	var itemDanger int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by)
		VALUES($1,'危险品','不明液体一瓶（约 500ml，有刺激性气味）','透明塑料瓶/无标签',$2,$3,$4,'driver',$5,'刘洋',
		'安保暂存区（不入柜）','贵重','危险品',3,$6,'危险品：不入普通柜，立即移交安保暂存区并报警联动；3 日内移交公安机关处理。',$7,'in_storage',$8) RETURNING id`,
		"WP"+now.Format("20060102")+"-0005", line10, vehs[3].id, ts(10, 0), uid["driver2"], retention(3), fleet2, uid["station2"]).Scan(&itemDanger)
	seedEvent(tx, nil, &itemDanger, uid["station2"], "station", "周杰", "站务登记入库", "危险品移交安保暂存区，已触发紧急报警")
	tx.Exec(`INSERT INTO alarms(item_id,type,level,title) VALUES($1,'danger','紧急','危险品上交：不明液体一瓶（10路 京B·D1001），已暂存安保区，请立即处置')`, itemDanger)
	seedAudit(tx, uid["station2"], "station", "周杰", "item", itemDanger, "register", "登记入安保暂存区 特殊=危险品")
	seedAudit(tx, uid["station2"], "station", "周杰", "item", itemDanger, "alarm", "触发危险品紧急报警")

	// 6. 雨伞（已逾期，待处置演示）
	var itemUmb int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by,created_at)
		VALUES($1,'其他','黑色长柄雨伞一把','伞骨完好',$2,$3,$4,'cleaner',NULL,'保洁-王阿姨',
		'D-11','普通','无',30,$5,'普通物品：保管期 30 天，逾期公告后捐赠或销毁。',$6,'expired',$7,$8) RETURNING id`,
		"WP"+now.AddDate(0, 0, -35).Format("20060102")+"-0006", line1, vehs[1].id, now.AddDate(0, 0, -35), now.AddDate(0, 0, -5), fleet1, uid["station"], now.AddDate(0, 0, -35)).Scan(&itemUmb)
	seedEvent(tx, nil, &itemUmb, uid["station"], "station", "李婷", "站务登记入库", "雨伞入柜 D-11，保管期 30 天")
	seedEvent(tx, nil, &itemUmb, 0, "system", "系统", "逾期", "超过保管期限，进入逾期待处置状态")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemUmb, "register", "登记入库 柜=D-11")
	seedAudit(tx, 0, "system", "系统", "item", itemUmb, "expire", "保管期满，系统自动标记逾期")

	// 6b. 过期药品（演示 药品→仅销毁 的类型处置规则）
	var itemMedExp int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,
		storage_cabinet,value_level,special_type,retention_days,retention_until,storage_rules,fleet_id,status,registered_by,created_at)
		VALUES($1,'药品','感冒灵颗粒一盒（已开封）','盒装/已开封',$2,$3,$4,'cleaner',NULL,'保洁-王阿姨',
		'冷藏柜-C1','普通','药品',7,$5,'药品：冷藏柜 2-8℃ 保管；保管期 7 天；逾期按医疗废弃物规范销毁（双人执行）。',$6,'expired',$7,$8) RETURNING id`,
		"WP"+now.AddDate(0, 0, -10).Format("20060102")+"-0008", line1, vehs[1].id, now.AddDate(0, 0, -10), now.AddDate(0, 0, -3), fleet1, uid["station"], now.AddDate(0, 0, -10)).Scan(&itemMedExp)
	seedEvent(tx, nil, &itemMedExp, uid["station"], "station", "李婷", "站务登记入库", "药品入冷藏柜 C1，保管期 7 天")
	seedEvent(tx, nil, &itemMedExp, 0, "system", "系统", "逾期", "超过保管期限，进入逾期待处置状态")
	seedAudit(tx, uid["station"], "station", "李婷", "item", itemMedExp, "register", "登记入库 柜=冷藏柜-C1 特殊=药品")
	seedAudit(tx, 0, "system", "系统", "item", itemMedExp, "expire", "保管期满，系统自动标记逾期")

	// 7. 钱包（司机刚上交，待站务登记）
	var itemWallet int
	tx.QueryRow(`INSERT INTO found_items(item_no,category,description,features,found_line_id,found_vehicle_id,found_at,handed_by_role,handed_by_id,handed_by_name,fleet_id,status)
		VALUES($1,'钱包','棕色皮质钱包，内有现金约 500 元及超市会员卡','棕色/皮质/现金约500元',$2,$3,$4,'driver',$5,'张建国',$6,'pending_register') RETURNING id`,
		"WP"+now.Format("20060102")+"-0007", line1, vehs[0].id, ts(11, 20), uid["driver"], fleet1).Scan(&itemWallet)
	seedEvent(tx, nil, &itemWallet, uid["driver"], "driver", "张建国", "司机上交", "1路 京A·D1001 座椅缝隙发现棕色钱包，已上交待登记")
	seedAudit(tx, uid["driver"], "driver", "张建国", "item", itemWallet, "handin", "上交物品待登记: 棕色皮质钱包")

	// 演示监控调阅申请（待安保审批）
	tx.Exec(`INSERT INTO surveillance_requests(report_id,requester_id,scope_type,vehicle_id,time_start,time_end,reason,involves_privacy)
		VALUES($1,$2,'vehicle',$3,$4,$5,'核查乘客下车时段车厢内情况，确认手机遗落位置',FALSE)`,
		reportID, uid["dispatcher"], vehs[0].id, ts(7, 40), ts(8, 40))
	seedEvent(tx, &reportID, nil, uid["dispatcher"], "dispatcher", "赵敏", "申请监控调阅", "申请调阅 京A·D1001 07:40-08:40 车厢监控，待安保审批")
	seedAudit(tx, uid["dispatcher"], "dispatcher", "赵敏", "surveillance", 1, "create", "创建调阅申请（车载 京A·D1001）")

	if err := tx.Commit(); err != nil {
		log.Fatalf("seed commit: %v", err)
	}
	log.Println("seed done")
}

func seedEvent(tx *sql.Tx, reportID, itemID *int, actorID int, role, name, action, detail string) {
	var aid any
	if actorID > 0 {
		aid = actorID
	}
	tx.Exec(`INSERT INTO events(report_id,item_id,actor_id,actor_role,actor_name,action,detail) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		reportID, itemID, aid, role, name, action, detail)
}

func seedAudit(tx *sql.Tx, actorID int, role, name, entityType string, entityID int, action, detail string) {
	var aid any
	if actorID > 0 {
		aid = actorID
	}
	tx.Exec(`INSERT INTO audit_logs(entity_type,entity_id,actor_id,actor_role,actor_name,action,detail) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		entityType, entityID, aid, role, name, action, detail)
}
