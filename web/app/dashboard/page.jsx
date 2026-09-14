'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../lib/api';
import { Badge, Card, Err, Loading } from '../../components/ui';
import {
  ROLES, REPORT_STATUS, REPORT_STATUS_COLOR, ITEM_STATUS, ITEM_STATUS_COLOR,
  SURV_STATUS, ALARM_TYPE, fmtDT, fmtD, toLocalInput,
} from '../../lib/util';

export default function Dashboard() {
  const [user, setUser] = useState(null);
  const [data, setData] = useState(null);
  const [alarms, setAlarms] = useState(null);
  const [meta, setMeta] = useState(null);
  const [err, setErr] = useState('');

  const refresh = () => {
    api('/api/dashboard').then(setData).catch((e) => setErr(e.message));
  };

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    refresh();
    api('/api/meta').then(setMeta).catch(() => {});
    if (['security', 'admin', 'station', 'cs', 'dispatcher'].includes(u.role)) {
      api('/api/alarms').then((d) => setAlarms(d.alarms)).catch(() => {});
    }
  }, []);

  if (!user) return null;
  if (!data) return <div className="container"><Loading /></div>;
  const c = data.counts || {};

  return (
    <div className="container">
      <Err msg={err} />

      {/* 统计卡片 */}
      <div className="grid grid-4 mb">
        {user.role === 'passenger' ? (
          <div className="stat"><div className="num">{(data.my_reports || []).length}</div><div className="lbl">我的申报</div></div>
        ) : (
          <>
            <div className="stat"><div className="num">{c.reports_submitted}</div><div className="lbl">待受理申报</div></div>
            <div className="stat"><div className="num">{c.reports_searching}</div><div className="lbl">排查中</div></div>
            <div className="stat"><div className="num">{c.items_in_storage}</div><div className="lbl">在库物品</div></div>
            <div className="stat warn"><div className="num">{c.items_expired}</div><div className="lbl">逾期待处置</div></div>
          </>
        )}
      </div>

      {user.role === 'passenger' && <PassengerPanel reports={data.my_reports || []} />}
      {(user.role === 'cs' || user.role === 'admin') && <CsPanel reports={data.active_reports || []} />}
      {user.role === 'driver' && <DriverPanel meta={meta} handins={data.my_handins || []} onDone={refresh} />}
      {(user.role === 'station' || user.role === 'admin') && (
        <StationPanel data={data} onDone={refresh} />
      )}
      {(user.role === 'dispatcher' || user.role === 'admin') && (
        <DispatcherPanel reports={data.searching_reports || []} />
      )}
      {(user.role === 'security' || user.role === 'admin') && (
        <SecurityPanel pending={data.surveillance_pending_list || []} alarms={alarms || []} onDone={() => {
          refresh();
          api('/api/alarms').then((d) => setAlarms(d.alarms)).catch(() => {});
        }} />
      )}
    </div>
  );
}

function ReportTable({ reports, showNext }) {
  if (reports.length === 0) return <div className="empty">暂无数据</div>;
  return (
    <table className="tbl">
      <thead><tr><th>单号</th><th>物品</th><th>线路/车辆</th><th>状态</th>{showNext && <th>下一步责任人</th>}<th>申报时间</th></tr></thead>
      <tbody>
        {reports.map((r) => (
          <tr key={r.id}>
            <td><Link href={`/reports/${r.id}`}>{r.report_no}</Link></td>
            <td>{r.category} · {r.description.slice(0, 20)}{r.description.length > 20 ? '…' : ''}</td>
            <td>{r.line_name || '—'} {r.plate_no ? `/ ${r.plate_no}` : r.vehicle_unknown ? '/ 车牌不详' : ''}</td>
            <td><Badge color={REPORT_STATUS_COLOR[r.status]}>{REPORT_STATUS[r.status]}</Badge></td>
            {showNext && <td>{r.next_role ? <Badge color="amber">{ROLES[r.next_role]}</Badge> : <span className="muted">—</span>}</td>}
            <td className="small muted">{fmtDT(r.created_at)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function PassengerPanel({ reports }) {
  return (
    <Card title="我的申报" extra={<Link className="btn btn-sm" href="/reports/new">+ 新建申报</Link>}>
      <ReportTable reports={reports} showNext />
      <div className="small muted mt">点击单号可查看查找进度、客服答复与认领入口。</div>
    </Card>
  );
}

function CsPanel({ reports }) {
  return (
    <Card title="客服工作台 · 进行中的招领单（含查找进度与下一步责任人）">
      <ReportTable reports={reports} showNext />
    </Card>
  );
}

function DispatcherPanel({ reports }) {
  return (
    <Card title="调度工作台 · 排查中的招领单">
      <ReportTable reports={reports} showNext={false} />
      <div className="small muted mt">进入招领单可使用 GPS / 刷卡记录 / 司机排班排查候选车辆，并申请监控调阅。</div>
    </Card>
  );
}

function DriverPanel({ meta, handins, onDone }) {
  const [form, setForm] = useState({
    category: '其他', description: '', features: '', found_vehicle_id: '',
    found_at: toLocalInput(new Date()), handed_by_role: 'driver', handed_by_name: '',
  });
  const [msg, setMsg] = useState('');
  const [err, setErr] = useState('');
  const set = (k, v) => setForm({ ...form, [k]: v });

  async function submit() {
    setErr(''); setMsg('');
    try {
      await api('/api/handin', {
        method: 'POST',
        body: {
          ...form,
          found_vehicle_id: form.found_vehicle_id ? Number(form.found_vehicle_id) : null,
          handed_by_name: form.handed_by_name || undefined,
        },
      });
      setMsg('已上交，等待站务登记入库');
      setForm({ ...form, description: '', features: '', handed_by_name: '' });
      onDone();
    } catch (e) { setErr(e.message); }
  }

  const vehicles = (meta && meta.vehicles) || [];
  return (
    <div className="grid grid-2">
      <Card title="司机/保洁上交物品">
        {msg && <div className="alert alert-ok">{msg}</div>}
        <Err msg={err} />
        <div className="form-row">
          <label className="f"><span>物品类别</span>
            <select className="in" value={form.category} onChange={(e) => set('category', e.target.value)}>
              {['手机', '钱包', '证件', '背包', '儿童物品', '银行卡', '药品', '危险品', '其他'].map((x) => <option key={x}>{x}</option>)}
            </select>
          </label>
          <label className="f"><span>发现车辆</span>
            <select className="in" value={form.found_vehicle_id} onChange={(e) => set('found_vehicle_id', e.target.value)}>
              <option value="">请选择</option>
              {vehicles.map((v) => <option key={v.id} value={v.id}>{v.plate_no}</option>)}
            </select>
          </label>
        </div>
        <label className="f"><span>物品描述</span>
          <textarea className="in" value={form.description} onChange={(e) => set('description', e.target.value)} placeholder="如：棕色皮质钱包，内有现金约500元" />
        </label>
        <div className="form-row">
          <label className="f"><span>发现时间</span>
            <input className="in" type="datetime-local" value={form.found_at} onChange={(e) => set('found_at', e.target.value)} />
          </label>
          <label className="f"><span>上交人</span>
            <select className="in" value={form.handed_by_role} onChange={(e) => set('handed_by_role', e.target.value)}>
              <option value="driver">司机</option>
              <option value="cleaner">保洁</option>
            </select>
          </label>
        </div>
        {form.handed_by_role === 'cleaner' && (
          <label className="f"><span>保洁姓名</span>
            <input className="in" value={form.handed_by_name} onChange={(e) => set('handed_by_name', e.target.value)} placeholder="如：保洁-王阿姨" />
          </label>
        )}
        <button className="btn" onClick={submit}>提交上交</button>
      </Card>
      <Card title="我的上交记录">
        {handins.length === 0 ? <div className="empty">暂无上交记录</div> : (
          <table className="tbl">
            <thead><tr><th>编号</th><th>物品</th><th>状态</th><th>时间</th></tr></thead>
            <tbody>
              {handins.map((it) => (
                <tr key={it.id}>
                  <td><Link href={`/items/${it.id}`}>{it.item_no}</Link></td>
                  <td>{it.description.slice(0, 18)}</td>
                  <td><Badge color={ITEM_STATUS_COLOR[it.status]}>{ITEM_STATUS[it.status]}</Badge></td>
                  <td className="small muted">{fmtDT(it.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}

function ItemMiniTable({ items, actionText }) {
  if (!items || items.length === 0) return <div className="empty">暂无</div>;
  return (
    <table className="tbl">
      <thead><tr><th>编号</th><th>物品</th><th>保管到期</th><th>状态</th><th></th></tr></thead>
      <tbody>
        {items.map((it) => (
          <tr key={it.id}>
            <td>{it.item_no}</td>
            <td>{it.category} · {it.description.slice(0, 16)}</td>
            <td className="small">{fmtD(it.retention_until)}</td>
            <td><Badge color={ITEM_STATUS_COLOR[it.status]}>{ITEM_STATUS[it.status]}</Badge></td>
            <td><Link className="btn btn-sm btn-secondary" href={it.status === 'pending_register' ? `/items/new?id=${it.id}` : `/items/${it.id}`}>{actionText}</Link></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function StationPanel({ data, onDone }) {
  const confirmTransfer = async (id) => {
    try { await api(`/api/transfers/${id}/confirm`, { method: 'POST' }); onDone(); }
    catch (e) { alert(e.message); }
  };
  return (
    <div className="grid grid-2">
      <Card title="待登记入库（司机/保洁已上交）">
        <ItemMiniTable items={data.items_pending_register || []} actionText="去登记" />
      </Card>
      <Card title="待核验 / 待签收认领单">
        {(data.claims_todo || []).length === 0 ? <div className="empty">暂无</div> : (
          <table className="tbl">
            <thead><tr><th>物品</th><th>认领人</th><th>状态</th><th></th></tr></thead>
            <tbody>
              {data.claims_todo.map((cl) => (
                <tr key={cl.id}>
                  <td>{cl.item_no} · {cl.item_description.slice(0, 12)}</td>
                  <td>{cl.claimant_name} {cl.claimant_phone}</td>
                  <td><Badge color={cl.status === 'pending' ? 'amber' : 'blue'}>{cl.status === 'pending' ? '待核验' : '待签收'}</Badge></td>
                  <td><Link className="btn btn-sm btn-secondary" href={`/items/${cl.item_id}`}>处理</Link></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="逾期待处置物品">
        <ItemMiniTable items={data.items_expired || []} actionText="去处置" />
      </Card>
      <Card title="即将到期（7 天内）">
        <ItemMiniTable items={data.items_expiring || []} actionText="查看" />
      </Card>
      {(data.transfers_todo || []).length > 0 && (
        <Card title="待确认接收的车队移交">
          <table className="tbl">
            <thead><tr><th>物品</th><th>来自车队</th><th>发起时间</th><th></th></tr></thead>
            <tbody>
              {data.transfers_todo.map((t) => (
                <tr key={t.id}>
                  <td>{t.item_no} · {t.item_description.slice(0, 14)}</td>
                  <td>{t.from_fleet}</td>
                  <td className="small muted">{fmtDT(t.created_at)}</td>
                  <td><button className="btn btn-sm" onClick={() => confirmTransfer(t.id)}>确认接收</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}

function SecurityPanel({ pending, alarms, onDone }) {
  const act = async (fn) => { try { await fn(); onDone(); } catch (e) { alert(e.message); } };
  return (
    <div className="grid grid-2">
      <Card title="监控调阅待审批">
        {pending.length === 0 ? <div className="empty">暂无待审批申请</div> : (
          <table className="tbl">
            <thead><tr><th>招领单</th><th>范围</th><th>时段</th><th>理由</th><th>隐私</th><th>操作</th></tr></thead>
            <tbody>
              {pending.map((s) => (
                <tr key={s.id}>
                  <td><Link href={`/reports/${s.report_id}`}>#{s.report_id}</Link></td>
                  <td>{s.scope_type === 'vehicle' ? `车载 ${s.plate_no}` : `站点 ${s.stop_name}`}</td>
                  <td className="small">{fmtDT(s.time_start)} ~ {fmtDT(s.time_end)}</td>
                  <td className="small">{s.reason}</td>
                  <td>{s.involves_privacy ? <Badge color="red">涉隐私</Badge> : '—'}</td>
                  <td className="btn-row">
                    <button className="btn btn-sm" onClick={() => act(() => api(`/api/surveillance/${s.id}/approve`, { method: 'POST' }))}>批准</button>
                    <button className="btn btn-sm btn-danger" onClick={() => {
                      const reason = prompt('驳回理由：');
                      if (reason !== null) act(() => api(`/api/surveillance/${s.id}/reject`, { method: 'POST', body: { reason } }));
                    }}>驳回</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="报警工单（贵重物品 / 危险品）">
        {alarms.filter((a) => a.status !== 'closed').length === 0 ? <div className="empty">暂无未关闭报警</div> : (
          <table className="tbl">
            <thead><tr><th>类型</th><th>级别</th><th>内容</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {alarms.filter((a) => a.status !== 'closed').map((a) => (
                <tr key={a.id}>
                  <td><Badge color={a.type === 'danger' ? 'red' : 'amber'}>{ALARM_TYPE[a.type]}</Badge></td>
                  <td>{a.level === '紧急' ? <Badge color="red">紧急</Badge> : '一般'}</td>
                  <td className="small">{a.title}</td>
                  <td><Badge color={a.status === 'open' ? 'red' : 'amber'}>{a.status === 'open' ? '未处理' : '跟进中'}</Badge></td>
                  <td className="btn-row">
                    {a.status === 'open' && <button className="btn btn-sm" onClick={() => act(() => api(`/api/alarms/${a.id}/ack`, { method: 'POST' }))}>确认</button>}
                    <button className="btn btn-sm btn-secondary" onClick={() => {
                      const notes = prompt('处置说明：');
                      if (notes !== null) act(() => api(`/api/alarms/${a.id}/close`, { method: 'POST', body: { notes } }));
                    }}>关闭</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}
