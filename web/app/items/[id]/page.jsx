'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { AuditList, Badge, Card, Err, Loading, Photos, Timeline, Uploader } from '../../../components/ui';
import { ITEM_STATUS, ITEM_STATUS_COLOR, CLAIM_STATUS, VERIFY_METHOD, DISPOSAL_ACTION, fmtDT, fmtD } from '../../../lib/util';

export default function ItemDetail({ params }) {
  const { id } = params;
  const [user, setUser] = useState(null);
  const [data, setData] = useState(null);
  const [meta, setMeta] = useState(null);
  const [err, setErr] = useState('');

  const refresh = () => api(`/api/items/${id}`).then(setData).catch((e) => setErr(e.message));

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

  const it = data.item;
  const isStation = ['station', 'admin'].includes(user.role);

  return (
    <div className="container">
      <div className="between mb">
        <h2 style={{ margin: 0, fontSize: 20 }}>物品 {it.item_no}</h2>
        <div className="flex">
          {it.value_level === '贵重' && <Badge color="amber">贵重</Badge>}
          {it.special_type !== '无' && <Badge color="red">{it.special_type}</Badge>}
          <Badge color={ITEM_STATUS_COLOR[it.status]}>{ITEM_STATUS[it.status]}</Badge>
        </div>
      </div>

      {it.special_type !== '无' && (
        <div className="alert alert-warn mb"><b>特殊保管规则：</b>{it.storage_rules}</div>
      )}

      <div className="grid grid-2">
        <div className="grid" style={{ alignContent: 'start' }}>
          <Card title="物品信息">
            <dl className="kv">
              <dt>类别</dt><dd>{it.category}</dd>
              <dt>描述</dt><dd>{it.description}</dd>
              <dt>特征</dt><dd>{it.features || '—'}</dd>
              <dt>照片</dt><dd><Photos urls={it.photos} /></dd>
              <dt>发现位置</dt><dd>{it.line_name || '—'} {it.plate_no || it.stop_name || ''} · {fmtDT(it.found_at)}</dd>
              <dt>上交人</dt><dd>{it.handed_by_role === 'cleaner' ? '保洁' : '司机'} · {it.handed_by_name || '—'}</dd>
              <dt>存放柜</dt><dd><b>{it.storage_cabinet || '—'}</b></dd>
              <dt>保管期限</dt><dd>{it.retention_days} 天（至 {fmtD(it.retention_until)}）</dd>
              <dt>保管车队</dt><dd>{it.fleet_name || '—'}</dd>
              {it.matched_report_id && (<><dt>关联招领单</dt><dd><Link href={`/reports/${it.matched_report_id}`}>#{it.matched_report_id}</Link></dd></>)}
            </dl>
          </Card>

          {data.alarms && data.alarms.length > 0 && (
            <Card title="联动报警">
              {data.alarms.map((a) => (
                <div key={a.id} className={`alert ${a.status === 'closed' ? 'alert-ok' : a.level === '紧急' ? 'alert-danger' : 'alert-warn'}`}>
                  <b>[{a.level}]</b> {a.title} <span className="muted small">（{a.status === 'closed' ? '已关闭' : a.status === 'ack' ? '跟进中' : '未处理'}）</span>
                </div>
              ))}
            </Card>
          )}

          {data.transfers && data.transfers.length > 0 && (
            <Card title="车队移交记录">
              <table className="tbl">
                <thead><tr><th>从</th><th>到</th><th>原因</th><th>状态</th><th>时间</th></tr></thead>
                <tbody>
                  {data.transfers.map((t) => (
                    <tr key={t.id}>
                      <td>{t.from_fleet}</td><td>{t.to_fleet}</td>
                      <td className="small">{t.reason || '—'}</td>
                      <td><Badge color={t.status === 'confirmed' ? 'green' : 'amber'}>{t.status === 'confirmed' ? '已确认' : '待确认'}</Badge></td>
                      <td className="small muted">{fmtDT(t.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}

          {data.disposals && data.disposals.length > 0 && (
            <Card title="逾期处置流程">
              {data.disposals.map((d) => (
                <div key={d.id} className="mb">
                  <div className="flex mb">
                    <Badge color="purple">{DISPOSAL_ACTION[d.action]}</Badge>
                    <span className="small muted">发起于 {fmtDT(d.created_at)}</span>
                    {d.executed && <Badge color="green">已执行</Badge>}
                  </div>
                  <div className="pipeline">
                    <span className={`step ${d.notice_done ? 'done' : ''}`}>① 公告{d.notice_done ? ' ✓' : ''}</span>
                    <span className={`step ${d.contact_done ? 'done' : ''}`}>② 联系乘客{d.contact_done ? ' ✓' : ''}</span>
                    <span className={`step ${d.confirm_done ? 'done' : ''}`}>③ 移交确认{d.confirm_done ? ' ✓' : ''}</span>
                    <span className={`step ${d.executed ? 'done' : ''}`}>④ 执行{d.executed ? ' ✓' : ''}</span>
                  </div>
                  {isStation && !d.executed && (
                    <div className="btn-row">
                      {!d.notice_done && <button className="btn btn-sm" onClick={() => step(d.id, 'notice')}>完成公告</button>}
                      {d.notice_done && !d.contact_done && <button className="btn btn-sm" onClick={() => step(d.id, 'contact')}>完成联系乘客</button>}
                      {d.contact_done && !d.confirm_done && <button className="btn btn-sm" onClick={() => step(d.id, 'confirm')}>完成移交确认</button>}
                      {d.confirm_done && <button className="btn btn-sm btn-danger" onClick={() => execute(d.id)}>执行{DISPOSAL_ACTION[d.action]}</button>}
                    </div>
                  )}
                </div>
              ))}
            </Card>
          )}
        </div>

        <div className="grid" style={{ alignContent: 'start' }}>
          <Card title="处理进度">
            <Timeline events={data.events} />
          </Card>

          {data.claims && data.claims.length > 0 && (
            <Card title="认领单">
              {data.claims.map((c) => (
                <div key={c.id} className="card" style={{ marginBottom: 10, background: '#f8fafc' }}>
                  <div className="between">
                    <b>{c.claimant_name}</b>
                    <Badge color={c.status === 'signed' ? 'green' : c.status === 'rejected' ? 'red' : 'amber'}>{CLAIM_STATUS[c.status]}</Badge>
                  </div>
                  <div className="small muted">{c.claimant_phone} · 证件 {c.claimant_id_card || '—'}</div>
                  <div className="small">核验方式：{VERIFY_METHOD[c.verify_method] || '待核验'}</div>
                  {c.delegate_name && <div className="small">代领人：{c.delegate_name}（{c.delegate_relation || '关系未填'}）证件 {c.delegate_id_card}</div>}
                  <ClaimCredential claimId={c.id} canView={['station', 'security', 'admin'].includes(user.role)} onViewed={refresh} />
                  {c.match_notes && <div className="small">核验备注:{c.match_notes}</div>}
                  {c.sign_photo && <div className="mt"><span className="small muted">签收照片：</span><Photos urls={[c.sign_photo]} /></div>}
                  {isStation && c.status === 'pending' && (
                    <ClaimVerify claimId={c.id} onDone={refresh} />
                  )}
                  {isStation && c.status === 'verified' && (
                    <ClaimSign claimId={c.id} onDone={refresh} />
                  )}
                </div>
              ))}
            </Card>
          )}

          {isStation && it.status === 'in_storage' && meta && (
            <TransferForm meta={meta} item={it} onDone={refresh} />
          )}
          {isStation && it.status === 'expired' && (data.disposals || []).filter((d) => !d.executed).length === 0 && (
            <DisposalForm item={it} onDone={refresh} />
          )}
        </div>
      </div>

      <div className="card mt">
        <h3><span className="dot" />审计链（上交 / 登记 / 认领 / 移交 / 处置全程留痕）</h3>
        <AuditList audits={data.audits} />
      </div>
    </div>
  );

  async function step(disposalId, s) {
    try { await api(`/api/disposals/${disposalId}/step`, { method: 'POST', body: { step: s } }); refresh(); }
    catch (e) { alert(e.message); }
  }
  async function execute(disposalId) {
    if (!confirm('确认执行处置？执行后物品状态将变更。')) return;
    try { await api(`/api/disposals/${disposalId}/execute`, { method: 'POST' }); refresh(); }
    catch (e) { alert(e.message); }
  }
}

function ClaimCredential({ claimId, canView, onViewed }) {
  const [cred, setCred] = useState(null);
  const [err, setErr] = useState('');
  if (!canView) return null;
  if (!cred) {
    return (
      <div className="mt">
        <button className="btn btn-sm btn-ghost" onClick={async () => {
          setErr('');
          try {
            setCred(await api(`/api/claims/${claimId}/credential`));
            onViewed && onViewed();
          } catch (e) { setErr(e.message); }
        }}>查看完整证件（授权核验，留审计）</button>
        {err && <div className="error-text">{err}</div>}
      </div>
    );
  }
  return (
    <div className="alert alert-info small mt">
      认领人证件：<b>{cred.claimant_id_card || '—'}</b>
      {cred.delegate_id_card && <>　代领人证件：<b>{cred.delegate_id_card}</b></>}
      <div className="muted">本次查看已记入审计链</div>
    </div>
  );
}

function ClaimVerify({ claimId, onDone }) {
  const [method, setMethod] = useState('id_card');
  const [notes, setNotes] = useState('');
  const [err, setErr] = useState('');
  return (
    <div className="mt">
      <Err msg={err} />
      <div className="flex">
        <select className="in" style={{ maxWidth: 160 }} value={method} onChange={(e) => setMethod(e.target.value)}>
          <option value="id_card">身份证核验</option>
          <option value="description">描述匹配</option>
          <option value="delegate">委托代领</option>
        </select>
        <input className="in" placeholder="核验备注（核对要点）" value={notes} onChange={(e) => setNotes(e.target.value)} />
      </div>
      <div className="btn-row">
        <button className="btn btn-sm" onClick={async () => {
          setErr('');
          try { await api(`/api/claims/${claimId}/verify`, { method: 'POST', body: { verify_method: method, match_notes: notes } }); onDone(); }
          catch (e) { setErr(e.message); }
        }}>核验通过</button>
        <button className="btn btn-sm btn-danger" onClick={async () => {
          const reason = prompt('驳回理由：');
          if (reason === null) return;
          try { await api(`/api/claims/${claimId}/reject`, { method: 'POST', body: { reason } }); onDone(); }
          catch (e) { alert(e.message); }
        }}>驳回</button>
      </div>
    </div>
  );
}

function ClaimSign({ claimId, onDone }) {
  const [photo, setPhoto] = useState('');
  const [err, setErr] = useState('');
  return (
    <div className="mt">
      <Err msg={err} />
      <div className="flex">
        <Uploader label="上传签收照片" onUploaded={setPhoto} />
        {photo && <img src={photo} className="img-thumb" alt="签收照片" />}
      </div>
      <div className="btn-row">
        <button className="btn btn-sm" disabled={!photo} onClick={async () => {
          setErr('');
          try { await api(`/api/claims/${claimId}/sign`, { method: 'POST', body: { sign_photo: photo } }); onDone(); }
          catch (e) { setErr(e.message); }
        }}>确认签收（物品出库）</button>
      </div>
    </div>
  );
}

function TransferForm({ meta, item, onDone }) {
  const [toFleet, setToFleet] = useState('');
  const [reason, setReason] = useState('');
  const [err, setErr] = useState('');
  const others = meta.fleets.filter((f) => f.id !== item.fleet_id);
  if (others.length === 0) return null;
  return (
    <Card title="车队间移交">
      <Err msg={err} />
      <label className="f"><span>目标车队</span>
        <select className="in" value={toFleet} onChange={(e) => setToFleet(e.target.value)}>
          <option value="">请选择</option>
          {others.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}
        </select>
      </label>
      <label className="f"><span>移交原因</span>
        <input className="in" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="如：乘客在另一车队线路遗失，统一移交处理" />
      </label>
      <button className="btn" disabled={!toFleet} onClick={async () => {
        setErr('');
        try { await api(`/api/items/${item.id}/transfer`, { method: 'POST', body: { to_fleet_id: Number(toFleet), reason } }); onDone(); }
        catch (e) { setErr(e.message); }
      }}>发起移交</button>
    </Card>
  );
}

function DisposalForm({ item, onDone }) {
  const allowed = {
    药品: ['destroy'], 银行卡: ['destroy'], 危险品: ['transfer_out'],
    儿童证件: ['transfer_out'], 身份证件: ['transfer_out'],
  }[item.special_type] || (item.value_level === '贵重' || ['手机', '钱包'].includes(item.category) ? ['transfer_out'] : ['donate', 'destroy']);
  const [action, setAction] = useState(allowed[0]);
  const [notes, setNotes] = useState('');
  const [err, setErr] = useState('');
  return (
    <Card title="发起逾期处置">
      <div className="alert alert-warn small">按物品类型，允许的处置方式：{allowed.map((a) => DISPOSAL_ACTION[a]).join(' / ')}；执行前需依次完成 公告 → 联系 → 移交确认。</div>
      <Err msg={err} />
      <label className="f"><span>处置方式</span>
        <select className="in" value={action} onChange={(e) => setAction(e.target.value)}>
          {allowed.map((a) => <option key={a} value={a}>{DISPOSAL_ACTION[a]}</option>)}
        </select>
      </label>
      <label className="f"><span>备注</span>
        <input className="in" value={notes} onChange={(e) => setNotes(e.target.value)} />
      </label>
      <button className="btn btn-danger" onClick={async () => {
        setErr('');
        try { await api(`/api/items/${item.id}/dispose`, { method: 'POST', body: { action, notes } }); onDone(); }
        catch (e) { setErr(e.message); }
      }}>发起处置流程</button>
    </Card>
  );
}
