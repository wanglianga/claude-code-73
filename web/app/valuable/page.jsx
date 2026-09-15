'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../lib/api';
import { Badge, Card, Err, Loading, Photos, Uploader } from '../../components/ui';
import { INTAKE_STATUS, fmtDT } from '../../lib/util';

export default function ValuablePage() {
  const [user, setUser] = useState(null);
  const [intakes, setIntakes] = useState(null);
  const [err, setErr] = useState('');

  const refresh = () => api('/api/valuable/intakes').then((d) => setIntakes(d.intakes || [])).catch((e) => setErr(e.message));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    refresh();
  }, []);

  if (!user) return null;
  const canCreate = ['station', 'admin'].includes(user.role);
  const canCountersign = ['security', 'admin'].includes(user.role);
  const todo = (intakes || []).filter((x) => x.status === 'pending_countersign');

  return (
    <div className="container">
      <Err msg={err} />
      <div className="flex mb" style={{ justifyContent: 'space-between' }}>
        <h2 style={{ margin: 0, fontSize: 20 }}>贵重物品双人入柜</h2>
        {canCreate && <Link className="btn" href="/valuable/new">+ 发起双人入柜</Link>}
      </div>
      <div className="alert alert-info">
        司机上交<b>手机 / 钱包 / 贵重物品</b>后，由<b>站务与安保共同拍照、封袋、入柜并记录柜号</b>；
        站务发起后由安保会签确认（双人不得为同一人，封袋破损不得入柜）。入柜后物品柜锁定、责任到人。
      </div>

      {canCountersign && (
        <Card title={`待会签入柜（${todo.length}）`} extra={<span className="small muted">安保核对封袋与柜号后会签</span>}>
          {todo.length === 0 ? <div className="empty">暂无待会签记录</div> : (
            <table className="tbl">
              <thead><tr><th>入柜单号</th><th>物品</th><th>柜号</th><th>封袋号</th><th>发起站务</th><th>时间</th><th></th></tr></thead>
              <tbody>
                {todo.map((x) => (
                  <tr key={x.id}>
                    <td>{x.intake_no}</td>
                    <td><Link href={`/items/${x.item_id}`}>{x.item_no}</Link> · {x.description.slice(0, 16)}</td>
                    <td><b>{x.cabinet_no}</b></td>
                    <td>{x.seal_no}</td>
                    <td>{x.station_name}</td>
                    <td className="small muted">{fmtDT(x.created_at)}</td>
                    <td><CountersignButton intake={x} onDone={refresh} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      <Card title="双人入柜记录" extra={<span className="small muted">共 {intakes ? intakes.length : 0} 条</span>}>
        {!intakes ? <Loading /> : intakes.length === 0 ? <div className="empty">暂无记录</div> : (
          <table className="tbl">
            <thead><tr><th>入柜单号</th><th>物品</th><th>柜号</th><th>封袋号/状态</th><th>站务</th><th>安保会签</th><th>状态</th><th>完成时间</th></tr></thead>
            <tbody>
              {intakes.map((x) => (
                <tr key={x.id}>
                  <td>{x.intake_no}</td>
                  <td><Link href={`/items/${x.item_id}`}>{x.item_no}</Link><div className="small muted">{x.category} · {x.description.slice(0, 18)}</div></td>
                  <td><b>{x.cabinet_no}</b></td>
                  <td>{x.seal_no}<div className="small">{x.seal_status === 'intact' ? '封袋完好' : x.seal_status}</div></td>
                  <td>{x.station_name}</td>
                  <td>{x.security_name || <Badge color="amber">待会签</Badge>}</td>
                  <td><Badge color={x.status === 'completed' ? 'green' : 'amber'}>{INTAKE_STATUS[x.status] || x.status}</Badge></td>
                  <td className="small muted">{x.completed_at ? fmtDT(x.completed_at) : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}

function CountersignButton({ intake, onDone }) {
  const [open, setOpen] = useState(false);
  const [photos, setPhotos] = useState([]);
  const [notes, setNotes] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  if (!open) return <button className="btn btn-sm" onClick={() => setOpen(true)}>核对会签</button>;
  return (
    <div style={{ minWidth: 260, background: '#f8fafc', border: '1px solid var(--border)', borderRadius: 8, padding: 10 }}>
      <div className="small muted mb">请现场核对：柜号 <b>{intake.cabinet_no}</b>、封袋 <b>{intake.seal_no}</b> 完好且与物品一致</div>
      <Err msg={err} />
      <div className="flex mb"><Photos urls={photos} /><Uploader label="补拍封袋/入柜照片" onUploaded={(u) => setPhotos([...photos, u])} /></div>
      <input className="in" placeholder="会签备注（可选）" value={notes} onChange={(e) => setNotes(e.target.value)} />
      <div className="btn-row">
        <button className="btn btn-sm" disabled={busy || photos.length === 0} onClick={async () => {
          setBusy(true); setErr('');
          try {
            await api(`/api/valuable/intakes/${intake.id}/countersign`, {
              method: 'POST', body: { security_photos: photos, seal_status: 'intact', notes },
            });
            setOpen(false); onDone();
          } catch (e) { setErr(e.message); } finally { setBusy(false); }
        }}>确认会签（双人入柜完成）</button>
        <button className="btn btn-sm btn-ghost" onClick={() => setOpen(false)}>取消</button>
      </div>
      {photos.length === 0 && <div className="small muted mt">需至少补拍 1 张会签照片</div>}
    </div>
  );
}
