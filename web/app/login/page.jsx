'use client';

import { useState } from 'react';
import { api, setUser } from '../../lib/api';

const DEMO_ACCOUNTS = [
  { u: 'passenger', label: '乘客 · 李明' },
  { u: 'passenger2', label: '乘客 · 王奶奶' },
  { u: 'cs', label: '客服 · 王晓' },
  { u: 'driver', label: '司机 · 张建国' },
  { u: 'station', label: '站务 · 李婷（交班）' },
  { u: 'station3', label: '站务 · 郑强（接班）' },
  { u: 'station2', label: '站务 · 周杰' },
  { u: 'manager', label: '站务主管 · 孙主任' },
  { u: 'dispatcher', label: '调度 · 赵敏' },
  { u: 'security', label: '安保 · 陈刚' },
  { u: 'admin', label: '管理员' },
];

export default function LoginPage() {
  const [username, setUsername] = useState('passenger');
  const [password, setPassword] = useState('123456');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e) {
    e && e.preventDefault();
    setErr('');
    setBusy(true);
    try {
      const data = await api('/api/auth/login', { method: 'POST', body: { username, password } });
      setUser(data.user);
      window.location.href = '/dashboard';
    } catch (e2) {
      setErr(e2.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login-wrap">
      <div className="card">
        <h3><span className="dot" />城市公交失物招领与监控调阅平台</h3>
        <form onSubmit={submit}>
          <label className="f">
            <span>用户名</span>
            <input className="in" value={username} onChange={(e) => setUsername(e.target.value)} autoFocus />
          </label>
          <label className="f">
            <span>密码</span>
            <input className="in" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <button className="btn" style={{ width: '100%' }} disabled={busy}>{busy ? '登录中…' : '登 录'}</button>
          {err && <div className="error-text">{err}</div>}
        </form>
        <div className="mt small muted">演示账号一键填充（密码均为 123456）：</div>
        <div className="login-quick">
          {DEMO_ACCOUNTS.map((a) => (
            <button key={a.u} className="btn btn-ghost btn-sm" onClick={() => { setUsername(a.u); setPassword('123456'); }}>
              {a.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
