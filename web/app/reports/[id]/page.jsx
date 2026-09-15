'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { AuditList, Badge, Card, Err, Loading, Timeline } from '../../../components/ui';
import {
  ROLES, REPORT_STATUS, REPORT_STATUS_COLOR, ITEM_STATUS, ITEM_STATUS_COLOR,
  CLAIM_STATUS, SURV_STATUS, VERIFY_METHOD, fmtDT,
} from '../../../lib/util';

const STEPS = ['submitted', 'searching', 'matched', 'claim_verifying', 'claimed'];

export default function ReportDetail({ params }) {
  const { id } = params;
  const [user, setUser] = useState(null);
  const [data, setData] = useState(null);
  const [meta, setMeta] = useState(null);
  const [err, setErr] = useState('');

  const refresh = () => api(`/api/reports/${id}`).then(setData).catch((e) => setErr(e.message));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    refresh();
    api('/api/meta').then(setMeta).catch(() => {});
  }, [id]);

  if (!user) return null;
  if (err && !data) return <div className="container"><Err msg={err} /></div>;
  if (!data) return <div className="container"><Loading /></div>;

  const r = data.report;
  const curStep = r.status === 'closed_unfound' ? -1 : STEPS.indexOf(r.status);

  return (
    <div className="container">
      <div className="between mb">
        <h2 style={{ margin: 0, fontSize: 20 }}>招领单 {r.report_no}</h2>
        <Badge color={REPORT_STATUS_COLOR[r.status]}>{REPORT_STATUS[r.status]}</Badge>
      </div>

      <div className="card mb">
        <div className="pipeline">
          {STEPS.map((s, i) => (
            <span key={s} className={`step ${i < curStep ? 'done' : i === curStep ? 'cur' : ''}`}>{REPORT_STATUS[s]}</span>
          ))}
          {r.status === 'closed_unfound' && <span className="step cur">未找到结案</span>}
        </div>
        {r.next_role && <div className="small muted mt">下一步责任角色：<Badge color="amber">{ROLES[r.next_role]}</Badge></div>}
      </div>

      <div className="grid grid-2">
        <div className="grid" style={{ alignContent: 'start' }}>
          <Card title="申报信息">
            <dl className="kv">
              <dt>物品类别</dt><dd>{r.category}</dd>
              <dt>物品描述</dt><dd>{r.description}</dd>
              <dt>物品特征</dt><dd>{r.features || '—'}</dd>
              <dt>线路</dt><dd>{r.line_name || '—'}</dd>
              <dt>车辆</dt><dd>{r.plate_no || (r.vehicle_unknown ? '车牌不详（待调度排查）' : '—')}</dd>
              <dt>上车 / 下车站</dt><dd>{r.board_stop || '—'} → {r.alight_stop || '—'}</dd>
              <dt>乘车时间</dt><dd>{fmtDT(r.ride_start)} ~ {fmtDT(r.ride_end)}</dd>
              <dt>座位位置</dt><dd>{r.seat_position || '—'}</dd>
              {r.is_transfer && (<><dt>换乘</dt><dd>换乘 {r.transfer_line_name || '—'}（{r.transfer_stop || '换乘站不详'}）</dd></>)}
              <dt>联系人</dt><dd>{r.contact_name} · {r.contact_phone}</dd>
              <dt>身份证件</dt><dd>
                {r.passenger_id_card || '—'} <span className="muted small">（脱敏展示）</span>
                <ReportCredential reportId={r.id} canView={['station', 'security', 'admin'].includes(user.role)} />
              </dd>
            </dl>
          </Card>

          {data.matched_item && (
            <Card title="已匹配物品" extra={<Link className="btn btn-sm btn-secondary" href={`/items/${data.matched_item.id}`}>查看物品</Link>}>
              {data.matched_item.claim_frozen && (
                <div className="alert alert-danger">
                  该贵重物品因<b>换班交接异常</b>已冻结认领、物品柜锁定，待站务主管复核完成后方可认领。
                </div>
              )}
              <dl className="kv">
                <dt>编号</dt><dd>{data.matched_item.item_no}</dd>
                <dt>描述</dt><dd>{data.matched_item.description}</dd>
                <dt>存放柜</dt><dd>{data.matched_item.storage_cabinet}</dd>
                <dt>保管状态</dt><dd><Badge color={ITEM_STATUS_COLOR[data.matched_item.status]}>{ITEM_STATUS[data.matched_item.status]}</Badge></dd>
                <dt>保管期限</dt><dd>{fmtDT(data.matched_item.retention_until).slice(0, 10)} 前</dd>
              </dl>
            </Card>
          )}

          {data.claims && data.claims.length > 0 && (
            <Card title="认领记录">
              <table className="tbl">
                <thead><tr><th>认领人</th><th>核验方式</th><th>状态</th><th>时间</th></tr></thead>
                <tbody>
                  {data.claims.map((c) => (
                    <tr key={c.id}>
                      <td>{c.claimant_name} {c.delegate_name ? `（代领：${c.delegate_name}）` : ''}</td>
                      <td>{VERIFY_METHOD[c.verify_method] || '—'}</td>
                      <td><Badge color={c.status === 'signed' ? 'green' : c.status === 'rejected' ? 'red' : 'amber'}>{CLAIM_STATUS[c.status]}</Badge></td>
                      <td className="small muted">{fmtDT(c.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}

          {data.surveillance && data.surveillance.length > 0 && (
            <Card title="监控调阅记录">
              <table className="tbl">
                <thead><tr><th>范围</th><th>时段</th><th>状态</th><th>结论</th></tr></thead>
                <tbody>
                  {data.surveillance.map((s) => (
                    <tr key={s.id}>
                      <td>{s.scope_type === 'vehicle' ? `车载 ${s.plate_no}` : `站点 ${s.stop_name}`}{s.involves_privacy && <> <Badge color="red">涉隐私</Badge></>}</td>
                      <td className="small">{fmtDT(s.time_start)} ~ {fmtDT(s.time_end)}</td>
                      <td><Badge color={s.status === 'approved' ? 'green' : s.status === 'rejected' ? 'red' : 'amber'}>{SURV_STATUS[s.status]}</Badge></td>
                      <td className="small">{s.result_notes || s.reject_reason || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}
        </div>

        <div className="grid" style={{ alignContent: 'start' }}>
          <Card title="查找进度 / 时间线">
            <Timeline events={data.events} />
          </Card>

          {user.role === 'cs' && <CsActions r={r} onDone={refresh} />}
          {user.role === 'admin' && <CsActions r={r} onDone={refresh} />}
          {user.role === 'dispatcher' && meta && <DispatcherActions r={r} meta={meta} surveillance={data.surveillance || []} onDone={refresh} />}
          {user.role === 'station' && r.status !== 'claimed' && r.status !== 'closed_unfound' && <StationMatch r={r} onDone={refresh} />}
          {user.role === 'passenger' && r.status === 'matched' && !(data.matched_item && data.matched_item.claim_frozen) && <ClaimForm user={user} reportId={r.id} onDone={refresh} />}
          {user.role === 'passenger' && r.status === 'matched' && data.matched_item && data.matched_item.claim_frozen && (
            <Card title="申请认领"><div className="alert alert-danger" style={{ margin: 0 }}>该物品柜因换班交接异常锁定、认领冻结中，待站务主管复核完成后将恢复认领，请稍后再试或联系客服。</div></Card>
          )}
        </div>
      </div>

      <div className="card mt">
        <h3><span className="dot" />审计链（查找 / 调阅 / 认领全程留痕）</h3>
        <AuditList audits={data.audits} />
      </div>
    </div>
  );
}

function ReportCredential({ reportId, canView }) {
  const [cred, setCred] = useState(null);
  const [err, setErr] = useState('');
  if (!canView) return null;
  if (!cred) {
    return (
      <>
        {' '}
        <button className="btn btn-sm btn-ghost" onClick={async () => {
          setErr('');
          try { setCred(await api(`/api/reports/${reportId}/credential`)); }
          catch (e) { setErr(e.message); }
        }}>查看原文</button>
        {err && <div className="error-text">{err}</div>}
      </>
    );
  }
  return (
    <div className="alert alert-info small mt">
      申报人证件原文：<b>{cred.passenger_id_card || '未填写'}</b>
      <span className="muted">（本次查看已记入审计链）</span>
    </div>
  );
}

function CsActions({ r, onDone }) {
  const [reply, setReply] = useState('');
  const [items, setItems] = useState([]);
  const [itemId, setItemId] = useState('');
  const [err, setErr] = useState('');

  useEffect(() => {
    if (r.status === 'searching') {
      api('/api/items?status=in_storage').then((d) => setItems(d.items || [])).catch(() => {});
    }
  }, [r.status]);

  const act = async (fn) => { setErr(''); try { await fn(); onDone(); } catch (e) { setErr(e.message); } };

  const accept = () => act(() => api(`/api/reports/${r.id}/accept`, { method: 'POST' }));
  const submitReply = () => act(async () => {
    await api(`/api/reports/${r.id}/reply`, { method: 'POST', body: { message: reply } });
    setReply('');
  });
  const matchItem = () => act(() => api(`/api/reports/${r.id}/match`, { method: 'POST', body: { item_id: Number(itemId) } }));
  const closeReport = () => {
    const reason = window.prompt('结案原因：', '多方查找未果，与乘客确认后结案');
    if (reason !== null) {
      act(() => api(`/api/reports/${r.id}/close`, { method: 'POST', body: { reason } }));
    }
  };

  return (
    <Card title="客服操作">
      <Err msg={err} />
      {r.status === 'submitted' && (
        <button className="btn" onClick={accept}>受理并转调度排查</button>
      )}
      {r.status === 'searching' && (
        <div className="mb">
          <label className="f"><span>匹配在库物品</span>
            <select className="in" value={itemId} onChange={(e) => setItemId(e.target.value)}>
              <option value="">选择物品…</option>
              {items.map((it) => <option key={it.id} value={it.id}>{it.item_no} · {it.category} · {it.description.slice(0, 20)}</option>)}
            </select>
          </label>
          <button className="btn" disabled={!itemId} onClick={matchItem}>确认匹配</button>
        </div>
      )}
      <label className="f"><span>答复乘客（乘客可在时间线看到）</span>
        <textarea className="in" value={reply} onChange={(e) => setReply(e.target.value)} placeholder="告知查找进度、物品保管状态与下一步安排…" />
      </label>
      <div className="btn-row">
        <button className="btn btn-secondary" disabled={!reply.trim()} onClick={submitReply}>提交答复</button>
        {['submitted', 'searching'].includes(r.status) && (
          <button className="btn btn-ghost" onClick={closeReport}>未找到结案</button>
        )}
      </div>
    </Card>
  );
}

function DispatcherActions({ r, meta, surveillance, onDone }) {
  const [note, setNote] = useState('');
  const [vehId, setVehId] = useState('');
  const [cand, setCand] = useState(null);
  const [candErr, setCandErr] = useState('');
  const [q, setQ] = useState({
    line_id: r.line_id || '', stop_id: '',
    start: (r.ride_start || '').slice(0, 16), end: (r.ride_end || '').slice(0, 16),
  });
  const [sv, setSv] = useState({ scope_type: 'vehicle', vehicle_id: '', stop_id: '', time_start: q.start, time_end: q.end, reason: '', involves_privacy: false });
  const [result, setResult] = useState({});
  const [err, setErr] = useState('');

  const line = meta.lines.find((l) => l.id === Number(q.line_id));
  const lineVehicles = meta.vehicles.filter((v) => v.line_id === Number(q.line_id));
  const act = async (fn) => { setErr(''); try { await fn(); onDone(); } catch (e) { setErr(e.message); } };

  async function findCandidates() {
    setCandErr('');
    try {
      const p = new URLSearchParams({ line_id: q.line_id, start: q.start, end: q.end });
      if (q.stop_id) p.set('stop_id', q.stop_id);
      setCand(await api(`/api/reports/${r.id}/candidates?${p}`));
    } catch (e) { setCandErr(e.message); }
  }

  const myApproved = surveillance.filter((s) => s.status === 'approved' && !s.result_notes);

  return (
    <>
      <Card title="调度排查（GPS / 刷卡 / 排班）">
        <Err msg={err} />
        <div className="form-row">
          <label className="f"><span>线路</span>
            <select className="in" value={q.line_id} onChange={(e) => setQ({ ...q, line_id: e.target.value })}>
              {meta.lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
            </select>
          </label>
          <label className="f"><span>限定站点（可选）</span>
            <select className="in" value={q.stop_id} onChange={(e) => setQ({ ...q, stop_id: e.target.value })}>
              <option value="">全部站点</option>
              {(line?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        </div>
        <div className="form-row">
          <label className="f"><span>开始时间</span><input className="in" type="datetime-local" value={q.start} onChange={(e) => setQ({ ...q, start: e.target.value })} /></label>
          <label className="f"><span>结束时间</span><input className="in" type="datetime-local" value={q.end} onChange={(e) => setQ({ ...q, end: e.target.value })} /></label>
        </div>
        <button className="btn btn-secondary" onClick={findCandidates}>查询候选车辆</button>
        {candErr && <div className="error-text">{candErr}</div>}
        {cand && (
          <div className="mt">
            <table className="tbl">
              <thead><tr><th>候选车辆</th><th>GPS 点数</th><th>刷卡笔数</th><th>途经站点</th></tr></thead>
              <tbody>
                {cand.candidates.map((x) => (
                  <tr key={x.vehicle_id}><td><b>{x.plate_no}</b></td><td>{x.gps_pings}</td><td>{x.card_swipes}</td><td className="small">{x.stops}</td></tr>
                ))}
              </tbody>
            </table>
            {(cand.shifts || []).length > 0 && (
              <div className="small muted mt">当班司机：{cand.shifts.map((s) => `${s.plate_no} → ${s.driver_name}（${s.start_time}-${s.end_time}）`).join('；')}</div>
            )}
          </div>
        )}
        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '14px 0' }} />
        <label className="f"><span>排查记录（写入时间线）</span>
          <textarea className="in" value={note} onChange={(e) => setNote(e.target.value)} placeholder="如：结合 GPS 与刷卡记录，锁定候选车辆 京A·D1001…" />
        </label>
        <div className="flex">
          <select className="in" style={{ maxWidth: 220 }} value={vehId} onChange={(e) => setVehId(e.target.value)}>
            <option value="">锁定车辆（可选）</option>
            {lineVehicles.map((v) => <option key={v.id} value={v.id}>{v.plate_no}</option>)}
          </select>
          <button className="btn" disabled={!note.trim()} onClick={() => act(async () => {
            await api(`/api/reports/${r.id}/dispatch-note`, { method: 'POST', body: { note, vehicle_id: vehId ? Number(vehId) : null } });
            setNote('');
          })}>提交排查记录</button>
        </div>
      </Card>

      <Card title="申请监控调阅（需安保审批）">
        <Err msg={err} />
        <div className="form-row">
          <label className="f"><span>调阅范围</span>
            <select className="in" value={sv.scope_type} onChange={(e) => setSv({ ...sv, scope_type: e.target.value })}>
              <option value="vehicle">车载监控</option>
              <option value="station">站点监控</option>
            </select>
          </label>
          {sv.scope_type === 'vehicle' ? (
            <label className="f"><span>车辆</span>
              <select className="in" value={sv.vehicle_id} onChange={(e) => setSv({ ...sv, vehicle_id: e.target.value })}>
                <option value="">请选择</option>
                {lineVehicles.map((v) => <option key={v.id} value={v.id}>{v.plate_no}</option>)}
              </select>
            </label>
          ) : (
            <label className="f"><span>站点</span>
              <select className="in" value={sv.stop_id} onChange={(e) => setSv({ ...sv, stop_id: e.target.value })}>
                <option value="">请选择</option>
                {(line?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </label>
          )}
        </div>
        <div className="form-row">
          <label className="f"><span>开始</span><input className="in" type="datetime-local" value={sv.time_start} onChange={(e) => setSv({ ...sv, time_start: e.target.value })} /></label>
          <label className="f"><span>结束</span><input className="in" type="datetime-local" value={sv.time_end} onChange={(e) => setSv({ ...sv, time_end: e.target.value })} /></label>
        </div>
        <label className="f"><span>调阅理由</span>
          <input className="in" value={sv.reason} onChange={(e) => setSv({ ...sv, reason: e.target.value })} placeholder="核查物品遗落位置" />
        </label>
        <label className="check">
          <input type="checkbox" checked={sv.involves_privacy} onChange={(e) => setSv({ ...sv, involves_privacy: e.target.checked })} />
          涉及身份证件等乘客隐私信息
        </label>
        <button className="btn" disabled={!sv.reason} onClick={() => act(() => api(`/api/reports/${r.id}/surveillance`, {
          method: 'POST',
          body: { ...sv, vehicle_id: sv.vehicle_id ? Number(sv.vehicle_id) : null, stop_id: sv.stop_id ? Number(sv.stop_id) : null },
        }))}>提交调阅申请</button>
      </Card>

      {myApproved.length > 0 && (
        <Card title="填写调阅结论">
          {myApproved.map((s) => (
            <div key={s.id} className="mb">
              <div className="small muted mb">{s.scope_type === 'vehicle' ? `车载 ${s.plate_no}` : `站点 ${s.stop_name}`} · {fmtDT(s.time_start)}~{fmtDT(s.time_end)}</div>
              <div className="flex">
                <input className="in" placeholder="调阅结论" value={result[s.id] || ''} onChange={(e) => setResult({ ...result, [s.id]: e.target.value })} />
                <button className="btn btn-sm" disabled={!(result[s.id] || '').trim()} onClick={() => act(() => api(`/api/surveillance/${s.id}/result`, { method: 'POST', body: { result: result[s.id] } }))}>提交</button>
              </div>
            </div>
          ))}
        </Card>
      )}
    </>
  );
}

function StationMatch({ r, onDone }) {
  const [items, setItems] = useState([]);
  const [itemId, setItemId] = useState('');
  const [err, setErr] = useState('');
  useEffect(() => {
    if (r.status === 'searching') api('/api/items?status=in_storage').then((d) => setItems(d.items || [])).catch(() => {});
  }, [r.status]);
  if (r.status !== 'searching') return null;
  return (
    <Card title="站务操作 · 匹配物品">
      <Err msg={err} />
      <label className="f"><span>在库物品</span>
        <select className="in" value={itemId} onChange={(e) => setItemId(e.target.value)}>
          <option value="">选择物品…</option>
          {items.map((it) => <option key={it.id} value={it.id}>{it.item_no} · {it.category} · {it.description.slice(0, 20)}</option>)}
        </select>
      </label>
      <button className="btn" disabled={!itemId} onClick={async () => {
        setErr('');
        try { await api(`/api/reports/${r.id}/match`, { method: 'POST', body: { item_id: Number(itemId) } }); onDone(); }
        catch (e) { setErr(e.message); }
      }}>确认匹配</button>
    </Card>
  );
}

function ClaimForm({ user, reportId, onDone }) {
  const [form, setForm] = useState({
    claimant_name: user.name || '', claimant_phone: '', claimant_id_card: '',
    verify_method: 'id_card', delegate_name: '', delegate_id_card: '', delegate_relation: '', match_notes: '',
  });
  const [err, setErr] = useState('');
  const set = (k, v) => setForm({ ...form, [k]: v });
  return (
    <Card title="申请认领">
      <Err msg={err} />
      <div className="form-row">
        <label className="f"><span>认领人姓名 *</span><input className="in" value={form.claimant_name} onChange={(e) => set('claimant_name', e.target.value)} /></label>
        <label className="f"><span>联系电话 *</span><input className="in" value={form.claimant_phone} onChange={(e) => set('claimant_phone', e.target.value)} /></label>
      </div>
      <label className="f"><span>核验方式</span>
        <select className="in" value={form.verify_method} onChange={(e) => set('verify_method', e.target.value)}>
          <option value="id_card">身份证核验</option>
          <option value="description">描述匹配</option>
          <option value="delegate">委托代领（如同学代领学生证）</option>
        </select>
      </label>
      {form.verify_method === 'id_card' && (
        <label className="f"><span>本人身份证号（脱敏存储）</span>
          <input className="in" value={form.claimant_id_card} onChange={(e) => set('claimant_id_card', e.target.value)} />
        </label>
      )}
      {form.verify_method === 'delegate' && (
        <>
          <div className="form-row">
            <label className="f"><span>被委托人姓名 *</span><input className="in" value={form.delegate_name} onChange={(e) => set('delegate_name', e.target.value)} /></label>
            <label className="f"><span>被委托人证件号 *</span><input className="in" value={form.delegate_id_card} onChange={(e) => set('delegate_id_card', e.target.value)} /></label>
          </div>
          <label className="f"><span>与失主关系</span><input className="in" value={form.delegate_relation} onChange={(e) => set('delegate_relation', e.target.value)} placeholder="如：同学 / 家属" /></label>
        </>
      )}
      <label className="f"><span>物品特征补充说明（供站务核验）</span>
        <textarea className="in" value={form.match_notes} onChange={(e) => set('match_notes', e.target.value)} placeholder="描述只有失主知道的细节…" />
      </label>
      <button className="btn" onClick={async () => {
        setErr('');
        try { await api(`/api/reports/${reportId}/claim`, { method: 'POST', body: form }); onDone(); }
        catch (e) { setErr(e.message); }
      }}>提交认领申请</button>
    </Card>
  );
}
