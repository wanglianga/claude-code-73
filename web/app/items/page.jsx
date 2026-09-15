'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../lib/api';
import { Badge, Card, Loading } from '../../components/ui';
import { ITEM_STATUS, ITEM_STATUS_COLOR, fmtD, fmtDT } from '../../lib/util';

export default function ItemsPage() {
  const [user, setUser] = useState(null);
  const [items, setItems] = useState(null);
  const [f, setF] = useState({ status: '', category: '', q: '', expiring: false });

  const load = () => {
    const p = new URLSearchParams();
    if (f.status) p.set('status', f.status);
    if (f.category) p.set('category', f.category);
    if (f.q) p.set('q', f.q);
    if (f.expiring) p.set('expiring', '1');
    api(`/api/items?${p}`).then((d) => setItems(d.items || [])).catch(() => setItems([]));
  };

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
  }, []);
  useEffect(() => { if (user) load(); }, [user, f.status, f.category, f.expiring]);

  if (!user) return null;

  return (
    <div className="container">
      <Card title="拾获物品库" extra={<span className="small muted">共 {items ? items.length : 0} 件</span>}>
        <div className="flex mb">
          <select className="in" style={{ maxWidth: 150 }} value={f.status} onChange={(e) => setF({ ...f, status: e.target.value })}>
            <option value="">全部状态</option>
            {Object.entries(ITEM_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
          <select className="in" style={{ maxWidth: 140 }} value={f.category} onChange={(e) => setF({ ...f, category: e.target.value })}>
            <option value="">全部类别</option>
            {['手机', '钱包', '证件', '背包', '儿童物品', '银行卡', '药品', '危险品', '其他'].map((x) => <option key={x}>{x}</option>)}
          </select>
          <input className="in" style={{ maxWidth: 220 }} placeholder="编号 / 描述关键字" value={f.q}
            onChange={(e) => setF({ ...f, q: e.target.value })} onKeyDown={(e) => e.key === 'Enter' && load()} />
          <button className="btn btn-sm btn-secondary" onClick={load}>查询</button>
          <label className="check" style={{ margin: 0 }}>
            <input type="checkbox" checked={f.expiring} onChange={(e) => setF({ ...f, expiring: e.target.checked })} />
            仅看 7 天内到期
          </label>
        </div>
        {!items ? <Loading /> : items.length === 0 ? <div className="empty">没有符合条件的物品</div> : (
          <table className="tbl">
            <thead><tr><th>编号</th><th>类别</th><th>描述</th><th>发现位置</th><th>存放柜</th><th>贵重</th><th>特殊</th><th>保管到期</th><th>状态</th></tr></thead>
            <tbody>
              {items.map((it) => (
                <tr key={it.id}>
                  <td><Link href={`/items/${it.id}`}>{it.item_no}</Link></td>
                  <td>{it.category}</td>
                  <td className="small">{it.description.slice(0, 22)}{it.description.length > 22 ? '…' : ''}</td>
                  <td className="small">{it.line_name || '—'} {it.plate_no || it.stop_name || ''}</td>
                  <td>{it.storage_cabinet || '—'}
                    {it.cabinet_locked && <div><Badge color="blue">🔒锁定</Badge></div>}
                    {it.claim_frozen && <div className="mt"><Badge color="red">❄认领冻结</Badge></div>}
                  </td>
                  <td>{it.value_level === '贵重' ? <Badge color="amber">贵重</Badge> : '普通'}</td>
                  <td>{it.special_type !== '无' ? <Badge color="red">{it.special_type}</Badge> : '—'}</td>
                  <td className="small">{fmtD(it.retention_until)}</td>
                  <td><Badge color={ITEM_STATUS_COLOR[it.status]}>{ITEM_STATUS[it.status]}</Badge></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}
