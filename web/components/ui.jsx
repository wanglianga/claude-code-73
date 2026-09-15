'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { getUser, logout, uploadImage } from '../lib/api';
import { ROLES, fmtDT } from '../lib/util';

export function Nav() {
  const [user, setUser] = useState(null);
  useEffect(() => { setUser(getUser()); }, []);
  if (!user) return null;
  const staff = ['cs', 'station', 'dispatcher', 'security', 'station_manager', 'admin'].includes(user.role);
  return (
    <nav className="nav">
      <Link className="brand" href="/dashboard">公交失物招领平台<span>Lost &amp; Found</span></Link>
      <Link className="navlink" href="/dashboard">工作台</Link>
      {(user.role === 'passenger' || user.role === 'cs' || user.role === 'admin') && (
        <Link className="navlink" href="/reports/new">申报遗失</Link>
      )}
      {staff && <Link className="navlink" href="/items">物品库</Link>}
      {(user.role === 'station' || user.role === 'admin') && (
        <Link className="navlink" href="/items/new">登记入库</Link>
      )}
      {(user.role === 'station' || user.role === 'security' || user.role === 'admin') && (
        <Link className="navlink" href="/valuable">贵重入柜</Link>
      )}
      {(user.role === 'station' || user.role === 'admin') && (
        <Link className="navlink" href="/handovers">换班交接</Link>
      )}
      {(user.role === 'station_manager' || user.role === 'admin') && (
        <Link className="navlink" href="/reviews">主管复核</Link>
      )}
      <div className="spacer" />
      <span className="who"><b>{user.name}</b>（{ROLES[user.role] || user.role}）</span>
      <button className="btn btn-sm btn-ghost" style={{ color: '#fff', borderColor: 'rgba(255,255,255,.4)' }} onClick={logout}>退出</button>
    </nav>
  );
}

export function Badge({ color = 'gray', children }) {
  return <span className={`badge badge-${color}`}>{children}</span>;
}

export function Card({ title, children, extra }) {
  return (
    <div className="card">
      {title && (
        <div className="card-title-row">
          <h3><span className="dot" />{title}</h3>
          {extra}
        </div>
      )}
      {children}
    </div>
  );
}

export function Timeline({ events }) {
  if (!events || events.length === 0) return <div className="empty">暂无进度记录</div>;
  return (
    <ul className="timeline">
      {events.map((e, i) => (
        <li key={i}>
          <div className="t-action">{e.action}</div>
          <div className="t-meta">{ROLES[e.actor_role] || e.actor_role} · {e.actor_name} · {fmtDT(e.created_at)}</div>
          {e.detail && <div className="t-detail">{e.detail}</div>}
        </li>
      ))}
    </ul>
  );
}

export function AuditList({ audits }) {
  if (!audits || audits.length === 0) return <div className="empty">暂无审计记录</div>;
  return (
    <table className="tbl">
      <thead><tr><th>时间</th><th>角色</th><th>操作人</th><th>动作</th><th>详情</th></tr></thead>
      <tbody>
        {audits.map((a, i) => (
          <tr key={i}>
            <td className="small muted" style={{ whiteSpace: 'nowrap' }}>{fmtDT(a.created_at)}</td>
            <td><Badge color="gray">{ROLES[a.actor_role] || a.actor_role}</Badge></td>
            <td>{a.actor_name}</td>
            <td>{a.action}</td>
            <td className="small">{a.detail}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function Uploader({ onUploaded, label = '上传图片' }) {
  const [busy, setBusy] = useState(false);
  return (
    <label className="btn btn-secondary btn-sm" style={{ cursor: 'pointer' }}>
      {busy ? '上传中…' : label}
      <input type="file" accept="image/*" hidden disabled={busy} onChange={async (e) => {
        const f = e.target.files && e.target.files[0];
        e.target.value = '';
        if (!f) return;
        setBusy(true);
        try { onUploaded(await uploadImage(f)); }
        catch (err) { alert(err.message); }
        finally { setBusy(false); }
      }} />
    </label>
  );
}

export function Photos({ urls }) {
  const [big, setBig] = useState(null);
  if (!urls || urls.length === 0) return <span className="muted small">无照片</span>;
  return (
    <div className="tag-list">
      {urls.map((u, i) => (
        <img key={i} src={u} className="img-thumb" alt={`照片${i + 1}`} onClick={() => setBig(u)} />
      ))}
      {big && (
        <div onClick={() => setBig(null)} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100, cursor: 'zoom-out' }}>
          <img src={big} className="img-preview" alt="预览" />
        </div>
      )}
    </div>
  );
}

export function Loading() { return <div className="empty">加载中…</div>; }

export function Err({ msg }) { return msg ? <div className="alert alert-danger">{msg}</div> : null; }
