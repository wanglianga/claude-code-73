'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../lib/api';
import { Badge, Card, Err, Loading } from '../../components/ui';
import { HANDOVER_STATUS, HANDOVER_STATUS_COLOR, fmtDT } from '../../lib/util';

export default function HandoversPage() {
  const [user, setUser] = useState(null);
  const [meta, setMeta] = useState(null);
  const [data, setData] = useState(null);
  const [toStation, setToStation] = useState('');
  const [err, setErr] = useState('');
  const [msg, setMsg] = useState('');

  const refresh = () => api('/api/handovers?scope=all').then((d) => setData(d.handovers || [])).catch((e) => setErr(e.message));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    api('/api/meta').then(setMeta).catch(() => {});
    refresh();
  }, []);

  if (!user) return null;
  if (!data) return <div className="container"><Loading /></div>;

  const myTodo = data.filter((h) => h.to_station_id === user.id && h.status === 'pending');
  const mine = data.filter((h) => h.from_station_id === user.id || h.to_station_id === user.id);
  const isStation = user.role === 'station';
  const stations = (meta?.staff || []).filter((s) => s.role === 'station' && s.id !== user.id);

  async function createHandover() {
    setErr(''); setMsg('');
    if (!toStation) { setErr('请选择接班站务'); return; }
    try {
      const d = await api('/api/handovers', { method: 'POST', body: { to_station_id: Number(toStation) } });
      setMsg(`交接单已发起，共 ${d.count} 件贵重物品，等待接班人逐项核对`);
      setToStation('');
      refresh();
    } catch (e) { setErr(e.message); }
  }

  return (
    <div className="container">
      <h2 style={{ margin: '0 0 12px', fontSize: 20 }}>站务换班交接（贵重物品）</h2>
      <div className="alert alert-info">
        换班时系统要求接班人<b>逐件核对柜号与封袋状态</b>；贵重物品始终责任到人、不能无人负责。
        核对异常时生成<b>站务主管复核任务</b>，物品柜保持锁定、认领自动冻结，主管复核闭环后方可解冻。
      </div>
      <Err msg={err} />
      {msg && <div className="alert alert-ok">{msg}</div>}

      {isStation && (
        <Card title="发起交班（我名下在柜贵重物品全部列入交接）">
          <div className="flex">
            <select className="in" style={{ maxWidth: 280 }} value={toStation} onChange={(e) => setToStation(e.target.value)}>
              <option value="">选择接班站务…</option>
              {stations.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
            <button className="btn" onClick={createHandover}>发起交接</button>
          </div>
        </Card>
      )}

      {isStation && myTodo.length > 0 && (
        <Card title={`待我接班核对（${myTodo.length}）`}>
          <table className="tbl">
            <thead><tr><th>交接单号</th><th>交班人</th><th>发起时间</th><th></th></tr></thead>
            <tbody>
              {myTodo.map((h) => (
                <tr key={h.id}>
                  <td>{h.handover_no}</td><td>{h.from_station}</td>
                  <td className="small muted">{fmtDT(h.created_at)}</td>
                  <td><Link className="btn btn-sm" href={`/handovers/${h.id}`}>逐项核对</Link></td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}

      <Card title={isStation ? '我的交接记录' : '全部交接记录'}>
        {mine.length === 0 ? <div className="empty">暂无交接记录</div> : (
          <table className="tbl">
            <thead><tr><th>交接单号</th><th>交班人</th><th>接班人</th><th>状态</th><th>异常数</th><th>核对说明 / 复核结论</th><th>时间</th><th></th></tr></thead>
            <tbody>
              {mine.map((h) => (
                <tr key={h.id}>
                  <td>{h.handover_no}</td>
                  <td>{h.from_station}</td>
                  <td>{h.to_station}</td>
                  <td><Badge color={HANDOVER_STATUS_COLOR[h.status]}>{HANDOVER_STATUS[h.status] || h.status}</Badge></td>
                  <td>{h.abnormal_count > 0 ? <Badge color="red">{h.abnormal_count}</Badge> : 0}</td>
                  <td className="small">{h.status === 'reviewed' || h.status === 'abnormal' ? (h.resolve_notes || h.check_notes) : (h.check_notes || '—')}</td>
                  <td className="small muted">{fmtDT(h.created_at)}</td>
                  <td>
                    {h.to_station_id === user.id && h.status === 'pending'
                      ? <Link className="btn btn-sm" href={`/handovers/${h.id}`}>去核对</Link>
                      : <Link className="btn btn-sm btn-secondary" href={`/handovers/${h.id}`}>查看</Link>}
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
