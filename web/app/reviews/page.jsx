'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../lib/api';
import { Badge, Card, Err, Loading } from '../../components/ui';
import { fmtDT } from '../../lib/util';

export default function ReviewsPage() {
  const [user, setUser] = useState(null);
  const [reviews, setReviews] = useState(null);
  const [meta, setMeta] = useState(null);
  const [err, setErr] = useState('');

  const refresh = () => api('/api/reviews').then((d) => setReviews(d.reviews || [])).catch((e) => setErr(e.message));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    refresh();
    api('/api/meta').then(setMeta).catch(() => {});
  }, []);

  if (!user) return null;
  if (!reviews) return <div className="container"><Loading /></div>;
  const pending = reviews.filter((r) => r.status === 'pending');
  const done = reviews.filter((r) => r.status !== 'pending');
  const stations = (meta?.staff || []).filter((s) => s.role === 'station');

  return (
    <div className="container">
      <h2 style={{ margin: '0 0 12px', fontSize: 20 }}>站务主管复核任务</h2>
      <div className="alert alert-info">
        换班交接核对异常时自动生成复核任务。复核期间<b>物品柜保持锁定、认领冻结</b>，贵重物品责任不中断。
        主管现场复核后：确认物品完好可<b>解除冻结</b>并重新指派责任人；确认异常属实则<b>维持冻结并紧急联动安保</b>。
      </div>
      <Err msg={err} />

      <Card title={`待复核（${pending.length}）`}>
        {pending.length === 0 ? <div className="empty">暂无待复核任务</div> : (
          <div className="grid">
            {pending.map((t) => (
              <ReviewCard key={t.id} task={t} stations={stations} onDone={refresh} />
            ))}
          </div>
        )}
      </Card>

      {done.length > 0 && (
        <Card title="历史复核">
          <table className="tbl">
            <thead><tr><th>任务</th><th>物品</th><th>结论</th><th>复核人</th><th>时间</th></tr></thead>
            <tbody>
              {done.map((t) => (
                <tr key={t.id}>
                  <td className="small">{t.title}</td>
                  <td>{t.item_no ? <Link href={`/items/${t.item_id}`}>{t.item_no}</Link> : '—'}</td>
                  <td className="small">{t.resolution}</td>
                  <td>{t.resolved_by}</td>
                  <td className="small muted">{t.resolved_at ? fmtDT(t.resolved_at) : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}

function ReviewCard({ task, stations, onDone }) {
  const [action, setAction] = useState('restore');
  const [resolution, setResolution] = useState('');
  const [custodian, setCustodian] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <div className="card" style={{ background: '#fffbeb', borderColor: '#fde68a' }}>
      <div className="between mb">
        <b>🔒 {task.title}</b>
        {task.item_id ? <Link className="btn btn-sm btn-secondary" href={`/items/${task.item_id}`}>查看物品</Link> : null}
      </div>
      <div className="small muted mb">{task.detail} · 发起于 {fmtDT(task.created_at)}</div>
      <Err msg={err} />
      <label className="f"><span>复核处置</span>
        <select className="in" value={action} onChange={(e) => setAction(e.target.value)}>
          <option value="restore">现场核对无误 / 已找回：解除冻结，恢复认领</option>
          <option value="escalate">异常属实：维持冻结，紧急联动安保上报</option>
        </select>
      </label>
      {action === 'restore' && (
        <label className="f"><span>恢复后责任人（默认接班人）</span>
          <select className="in" value={custodian} onChange={(e) => setCustodian(e.target.value)}>
            <option value="">默认（交接接班人）</option>
            {stations.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </label>
      )}
      <label className="f"><span>复核结论 *</span>
        <textarea className="in" value={resolution} onChange={(e) => setResolution(e.target.value)} placeholder="现场复核情况、封袋/柜号核对结果、处置说明" />
      </label>
      <button className={action === 'escalate' ? 'btn btn-danger' : 'btn'} disabled={busy || !resolution.trim()} onClick={async () => {
        setBusy(true); setErr('');
        try {
          await api(`/api/reviews/${task.id}/resolve`, {
            method: 'POST',
            body: { action, resolution, new_custodian_id: custodian ? Number(custodian) : null },
          });
          onDone();
        } catch (e) { setErr(e.message); } finally { setBusy(false); }
      }}>{busy ? '提交中…' : '提交复核结论'}</button>
    </div>
  );
}
