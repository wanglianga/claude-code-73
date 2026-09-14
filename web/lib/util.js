export const ROLES = {
  passenger: '乘客', cs: '客服', driver: '司机', station: '站务',
  dispatcher: '调度', security: '安保', admin: '管理员', system: '系统',
};

export const REPORT_STATUS = {
  submitted: '待受理', searching: '排查中', matched: '已匹配',
  claim_verifying: '认领核验中', claimed: '已认领完结', closed_unfound: '未找到结案',
};

export const REPORT_STATUS_COLOR = {
  submitted: 'amber', searching: 'blue', matched: 'purple',
  claim_verifying: 'amber', claimed: 'green', closed_unfound: 'gray',
};

export const ITEM_STATUS = {
  pending_register: '待登记', in_storage: '在库保管', matched: '已匹配',
  claimed: '已认领出库', expired: '逾期待处置', transferred: '已移交',
  destroyed: '已销毁', donated: '已捐赠',
};

export const ITEM_STATUS_COLOR = {
  pending_register: 'amber', in_storage: 'blue', matched: 'purple',
  claimed: 'green', expired: 'red', transferred: 'gray', destroyed: 'gray', donated: 'teal',
};

export const CLAIM_STATUS = { pending: '待核验', verified: '已核验待签收', signed: '已签收', rejected: '已驳回' };
export const SURV_STATUS = { pending: '待审批', approved: '已批准', rejected: '已拒绝' };
export const VERIFY_METHOD = { id_card: '身份证核验', description: '描述匹配', delegate: '委托代领' };
export const DISPOSAL_ACTION = { transfer_out: '移交公安机关', destroy: '销毁', donate: '捐赠' };
export const ALARM_TYPE = { valuable: '贵重物品', danger: '危险品' };

// 门店本地时区：全站时间统一按此时区展示，与浏览器所在时区无关
export const STORE_TZ = 'Asia/Shanghai';

function tzParts(d) {
  const fmt = new Intl.DateTimeFormat('en-CA', {
    timeZone: STORE_TZ, year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  });
  const o = {};
  for (const p of fmt.formatToParts(d)) o[p.type] = p.value;
  return o;
}

export function fmtDT(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d)) return s;
  const p = tzParts(d);
  return `${p.year}-${p.month}-${p.day} ${p.hour}:${p.minute}`;
}

export function fmtD(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d)) return s;
  const p = tzParts(d);
  return `${p.year}-${p.month}-${p.day}`;
}

// datetime-local 输入值
export function toLocalInput(d) {
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
