-- 城市公交失物招领与监控调阅平台 数据库结构
CREATE TABLE IF NOT EXISTS fleets (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS lines (
  id SERIAL PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  fleet_id INT NOT NULL REFERENCES fleets(id)
);

CREATE TABLE IF NOT EXISTS stops (
  id SERIAL PRIMARY KEY,
  line_id INT NOT NULL REFERENCES lines(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  seq INT NOT NULL
);

CREATE TABLE IF NOT EXISTS vehicles (
  id SERIAL PRIMARY KEY,
  plate_no TEXT NOT NULL UNIQUE,
  line_id INT NOT NULL REFERENCES lines(id),
  fleet_id INT NOT NULL REFERENCES fleets(id)
);

CREATE TABLE IF NOT EXISTS users (
  id SERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL, -- passenger/cs/driver/station/dispatcher/security/admin
  name TEXT NOT NULL,
  phone TEXT,
  fleet_id INT REFERENCES fleets(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 司机排班
CREATE TABLE IF NOT EXISTS driver_shifts (
  id SERIAL PRIMARY KEY,
  driver_id INT NOT NULL REFERENCES users(id),
  vehicle_id INT NOT NULL REFERENCES vehicles(id),
  shift_date DATE NOT NULL,
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL
);

-- 车辆 GPS 轨迹（调度排查用）
CREATE TABLE IF NOT EXISTS gps_pings (
  id BIGSERIAL PRIMARY KEY,
  vehicle_id INT NOT NULL REFERENCES vehicles(id),
  ts TIMESTAMPTZ NOT NULL,
  stop_id INT REFERENCES stops(id),
  lat NUMERIC(9,5),
  lng NUMERIC(9,5)
);
CREATE INDEX IF NOT EXISTS idx_gps_vehicle_ts ON gps_pings(vehicle_id, ts);

-- 刷卡记录（调度排查用）
CREATE TABLE IF NOT EXISTS card_swipes (
  id BIGSERIAL PRIMARY KEY,
  vehicle_id INT NOT NULL REFERENCES vehicles(id),
  card_no TEXT NOT NULL,
  ts TIMESTAMPTZ NOT NULL,
  stop_id INT REFERENCES stops(id)
);
CREATE INDEX IF NOT EXISTS idx_swipe_vehicle_ts ON card_swipes(vehicle_id, ts);

-- 失物招领单（乘客遗失申报）
CREATE TABLE IF NOT EXISTS lost_reports (
  id SERIAL PRIMARY KEY,
  report_no TEXT NOT NULL UNIQUE,
  passenger_id INT REFERENCES users(id),
  category TEXT NOT NULL,            -- 手机/钱包/证件/背包/儿童物品/银行卡/药品/危险品/其他
  description TEXT NOT NULL,
  features TEXT,                     -- 物品特征
  line_id INT REFERENCES lines(id),
  vehicle_id INT REFERENCES vehicles(id), -- 可空：老人/乘客记不清车牌
  vehicle_unknown BOOLEAN NOT NULL DEFAULT FALSE,
  board_stop_id INT REFERENCES stops(id),
  alight_stop_id INT REFERENCES stops(id),
  ride_start TIMESTAMPTZ,
  ride_end TIMESTAMPTZ,
  seat_position TEXT,
  is_transfer BOOLEAN NOT NULL DEFAULT FALSE, -- 跨线路换乘后遗失
  transfer_line_id INT REFERENCES lines(id),
  transfer_stop_id INT REFERENCES stops(id),
  contact_name TEXT NOT NULL,
  contact_phone TEXT NOT NULL,
  passenger_id_card TEXT,            -- 身份证件号（隐私，接口脱敏）
  status TEXT NOT NULL DEFAULT 'submitted', -- submitted/searching/matched/claim_verifying/claimed/closed_unfound
  next_role TEXT NOT NULL DEFAULT 'cs',     -- 下一步责任角色
  assigned_cs_id INT REFERENCES users(id),
  matched_item_id INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 拾获物品（司机/保洁上交，站务登记入库）
CREATE TABLE IF NOT EXISTS found_items (
  id SERIAL PRIMARY KEY,
  item_no TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL,
  description TEXT NOT NULL,
  features TEXT,
  photos TEXT[] NOT NULL DEFAULT '{}',
  found_line_id INT REFERENCES lines(id),
  found_vehicle_id INT REFERENCES vehicles(id),
  found_stop_id INT REFERENCES stops(id),
  found_at TIMESTAMPTZ,
  handed_by_role TEXT,               -- driver/cleaner
  handed_by_id INT REFERENCES users(id),
  handed_by_name TEXT,
  storage_cabinet TEXT,
  value_level TEXT NOT NULL DEFAULT '普通', -- 普通/贵重
  special_type TEXT NOT NULL DEFAULT '无',  -- 无/银行卡/药品/儿童证件/危险品
  retention_days INT NOT NULL DEFAULT 30,
  retention_until DATE,
  storage_rules TEXT,                -- 触发的保管规则与告知流程说明
  fleet_id INT REFERENCES fleets(id), -- 当前保管车队
  status TEXT NOT NULL DEFAULT 'pending_register',
  -- pending_register/in_storage/matched/claimed/expired/transferred/destroyed/donated
  matched_report_id INT REFERENCES lost_reports(id),
  registered_by INT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 认领单
CREATE TABLE IF NOT EXISTS claims (
  id SERIAL PRIMARY KEY,
  item_id INT NOT NULL REFERENCES found_items(id),
  report_id INT REFERENCES lost_reports(id),
  claimant_name TEXT NOT NULL,
  claimant_phone TEXT NOT NULL,
  claimant_id_card TEXT,             -- 脱敏存储
  verify_method TEXT,                -- id_card/description/delegate
  delegate_name TEXT,                -- 委托代领：被委托人
  delegate_id_card TEXT,
  delegate_relation TEXT,            -- 与失主关系（如同学）
  match_notes TEXT,
  sign_photo TEXT,
  status TEXT NOT NULL DEFAULT 'pending', -- pending/verified/signed/rejected
  submitted_by INT REFERENCES users(id),
  verified_by INT REFERENCES users(id),
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 监控调阅申请（调度发起，安保审批）
CREATE TABLE IF NOT EXISTS surveillance_requests (
  id SERIAL PRIMARY KEY,
  report_id INT NOT NULL REFERENCES lost_reports(id),
  requester_id INT NOT NULL REFERENCES users(id),
  scope_type TEXT NOT NULL,          -- vehicle/station
  vehicle_id INT REFERENCES vehicles(id),
  stop_id INT REFERENCES stops(id),
  time_start TIMESTAMPTZ NOT NULL,
  time_end TIMESTAMPTZ NOT NULL,
  reason TEXT NOT NULL,
  involves_privacy BOOLEAN NOT NULL DEFAULT FALSE, -- 涉及身份证件等隐私
  status TEXT NOT NULL DEFAULT 'pending', -- pending/approved/rejected
  approver_id INT REFERENCES users(id),
  approved_at TIMESTAMPTZ,
  reject_reason TEXT,
  result_notes TEXT,                 -- 调阅结论
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 车队之间移交
CREATE TABLE IF NOT EXISTS transfers (
  id SERIAL PRIMARY KEY,
  item_id INT NOT NULL REFERENCES found_items(id),
  from_fleet_id INT NOT NULL REFERENCES fleets(id),
  to_fleet_id INT NOT NULL REFERENCES fleets(id),
  reason TEXT,
  status TEXT NOT NULL DEFAULT 'pending', -- pending/confirmed
  initiated_by INT REFERENCES users(id),
  confirmed_by INT REFERENCES users(id),
  confirmed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 逾期处置（公告→联系→移交确认→执行）
CREATE TABLE IF NOT EXISTS disposals (
  id SERIAL PRIMARY KEY,
  item_id INT NOT NULL REFERENCES found_items(id),
  action TEXT NOT NULL,              -- transfer_out(移交公安)/destroy(销毁)/donate(捐赠)
  notice_done BOOLEAN NOT NULL DEFAULT FALSE,
  notice_at TIMESTAMPTZ, notice_by INT REFERENCES users(id),
  contact_done BOOLEAN NOT NULL DEFAULT FALSE,
  contact_at TIMESTAMPTZ, contact_by INT REFERENCES users(id),
  confirm_done BOOLEAN NOT NULL DEFAULT FALSE,
  confirm_at TIMESTAMPTZ, confirm_by INT REFERENCES users(id),
  executed BOOLEAN NOT NULL DEFAULT FALSE,
  executed_at TIMESTAMPTZ, executed_by INT REFERENCES users(id),
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 报警工单（贵重物品/危险品联动安保）
CREATE TABLE IF NOT EXISTS alarms (
  id SERIAL PRIMARY KEY,
  item_id INT REFERENCES found_items(id),
  report_id INT REFERENCES lost_reports(id),
  type TEXT NOT NULL,                -- valuable/danger
  level TEXT NOT NULL DEFAULT '一般', -- 一般/紧急
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open', -- open/ack/closed
  handled_by INT REFERENCES users(id),
  handled_at TIMESTAMPTZ,
  handle_notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 业务时间线（招领单/物品进度，客服答复乘客可见）
CREATE TABLE IF NOT EXISTS events (
  id BIGSERIAL PRIMARY KEY,
  report_id INT REFERENCES lost_reports(id),
  item_id INT REFERENCES found_items(id),
  actor_id INT,
  actor_role TEXT,
  actor_name TEXT,
  action TEXT NOT NULL,
  detail TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_report ON events(report_id);
CREATE INDEX IF NOT EXISTS idx_events_item ON events(item_id);

-- 审计链：所有查找、调阅、上交、认领、处置记录
CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  entity_id INT NOT NULL,
  actor_id INT,
  actor_role TEXT,
  actor_name TEXT,
  action TEXT NOT NULL,
  detail TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type, entity_id);
