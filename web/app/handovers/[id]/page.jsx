'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { Badge, Card, Err, Loading } from '../../../components/ui';
import { HANDOVER_STATUS, HANDOVER_STATUS_COLOR, CHECK_RESULT, CHECK_RESULT_COLOR, fmtDT } from '../../../lib/util';

const RESULT_OPTIONS = [
  { v: 'ok', label: '正常（柜号一致、封袋完好、物品在位）' },
  { v: 'mismatch', label: '异常：柜号不符' },
  { v: 'damaged', label: '异常：封袋破损' },
  { v: 'missing', label: '异常：物品缺失' },
];

export default function HandoverDetail({ params }) {
  const { id } = params;
  const [user, setUser] = useState(null);
  const [h, setH] = useState(null);
  const [err, setErr] = useState('');
  const [results, setResults] = useState({}); // item_id -> {result, notes}
  const [busy, setBusy] = useState(false);

  const refresh = () => api(`/api/handovers/${id}`).then((d) => {
    setH(d);
    const init = {};
    (d.items || []).forEach((it) => { init[it.item_id] = { result: it.check_result === 'pending' ? '' : it.check_result, notes: it.notes || '' }; });
    setResults(init);
  }).catch((e) => setErr(e.message));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    refresh();
  }, [id]);

  if (!user) return null;
  if (err && !h) return <div className="container"><Err msg={err} /></div>;
  if (!h) return <div className="container"><Loading /></div>;

  const canCheck = h.to_station_id === user.id && h.status === 'pending';
  const allChecked = h.items.length > 0 && h.items.every((it) => results[it.item_id]?.result);
  const hasAbnormal = Object.values(results).some((r) => r.result && r.result !== 'ok');

  async function submit() {
    setErr('');
    if (hasAbnormal) {
      if (!confirm('核对存在异常项。\n确认提交？提交后异常物品的物品柜将保持锁定、认领冻结，并自动生成站务主管复核任务。')) return;
    }
    setBusy(true);
    try {
      const payload = Object.entries(results).map(([itemId, r]) => ({ item_id: Number(itemId), result: r.result, notes: r.notes }));
      await api(`/api/handovers/${id}/check`, { method: 'POST', body: { results: payload } });
      await refresh();
    } catch (e) { setErr(e.message); } finally { setBusy(false); }
  }

  return (
    <div className="container" style={{ maxWidth: 900 }}>
      <div className="between mb">
        <h2 style={{ margin: 0, fontSize: 20 }}>换班交接单 {h.handover_no}</h2>
        <Badge color={HANDOVER_STATUS_COLOR[h.status]}>{HANDOVER_STATUS[h.status] || h.status}</Badge>
      </div>
      <div className="card mb">
        <dl className="kv">
          <dt>交班人</dt><dd>{h.from_station}</dd>
          <dt>接班人</dt><dd>{h.to_station}（交接人逐项核对柜号与封袋状态）</dd>
          <dt>班次日期</dt><dd>{h.shift_date}</dd>
          {h.check_notes && <><dt>核对说明</dt><dd>{h.check_notes}</dd></>}
          {h.resolve_notes && <><dt>主管复核结论</dt><dd>{h.resolve_notes}</dd></>}
        </dl>
      </div>
      <Err msg={err} />
      <Card title={`逐项核对（${h.items.length} 件贵重物品，不得遗漏）`}>
        <table className="tbl">
          <thead><tr><th>物品</th><th>登记柜号 / 封袋号</th><th>接班人核对结果</th><th>备注</th></tr></thead>
          <tbody>
            {h.items.map((it) => {
              const r = results[it.item_id] || { result: '', notes: '' };
              const done = it.check_result !== 'pending';
              return (
                <tr key={it.id}>
                  <td><Link href={`/items/${it.item_id}`}>{it.item_no}</Link><div className="small muted">{it.description}</div></td>
                  <td><b>{it.cabinet_no}</b><div className="small muted">封袋 {it.seal_no || '—'}</div></td>
                  <td>
                    {canCheck ? (
                      <select className="in" value={r.result} onChange={(e) => setResults({ ...results, [it.item_id]: { ...r, result: e.target.value } })}>
                        <option value="" disabled>请现场核对后选择…</option>
                        {RESULT_OPTIONS.map((o) => <option key={o.v} value={o.v}>{o.label}</option>)}
                      </select>
                    ) : (
                      <Badge color={CHECK_RESULT_COLOR[it.check_result]}>{CHECK_RESULT[it.check_result] || it.check_result}</Badge>
                    )}
                  </td>
                  <td>
                    {canCheck ? (
                      <input className="in" placeholder="异常情况说明" value={r.notes} onChange={(e) => setResults({ ...results, [it.item_id]: { ...r, notes: e.target.value } })} />
                    ) : it.notes || '—'}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {canCheck && (
          <>
            {hasAbnormal && (
              <div className="alert alert-danger mt">
                存在异常项：提交后异常物品的<b>物品柜保持锁定、认领自动冻结</b>，系统将向<b>站务主管</b>下发复核任务；
                异常物品责任人暂挂交班人，贵重物品不能无人负责。
              </div>
            )}
            <div className="btn-row">
              <button className="btn" disabled={busy || !allChecked} onClick={submit}>{busy ? '提交中…' : '提交核对结果'}</button>
              {!allChecked && <span className="small muted">还有物品未核对</span>}
            </div>
          </>
        )}
        {h.status === 'abnormal' && (
          <div className="alert alert-warn mt">该交接存在异常，已生成站务主管复核任务；复核完成前相关物品柜锁定、认领冻结。</div>
        )}
        {h.status === 'reviewed' && <div className="alert alert-ok mt">主管已完成复核，交接闭环。</div>}
        {h.status === 'normal' && <div className="alert alert-ok mt">逐项核对正常，责任已转移给接班人；物品柜保持锁定保管。</div>}
      </Card>
    </div>
  );
}
