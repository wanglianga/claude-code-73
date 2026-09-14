'use client';

import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { Card, Err } from '../../../components/ui';
import { toLocalInput } from '../../../lib/util';

export default function NewReport() {
  const [user, setUser] = useState(null);
  const [meta, setMeta] = useState(null);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const now = new Date();
  const [form, setForm] = useState({
    category: '手机', description: '', features: '',
    line_id: '', vehicle_id: '', vehicle_unknown: false,
    board_stop_id: '', alight_stop_id: '',
    ride_start: toLocalInput(new Date(now.getTime() - 3600e3)), ride_end: toLocalInput(now),
    seat_position: '', is_transfer: false, transfer_line_id: '', transfer_stop_id: '',
    contact_name: '', contact_phone: '', passenger_id_card: '',
  });
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    set('contact_name', u.name || '');
    api('/api/meta').then(setMeta).catch((e) => setErr(e.message));
  }, []);

  if (!user || !meta) return <div className="container"><div className="empty">加载中…</div></div>;

  const line = meta.lines.find((l) => String(l.id) === String(form.line_id));
  const tLine = meta.lines.find((l) => String(l.id) === String(form.transfer_line_id));
  const lineVehicles = meta.vehicles.filter((v) => String(v.line_id) === String(form.line_id));

  async function submit() {
    setErr('');
    if (!form.line_id) { setErr('请选择乘坐线路'); return; }
    setBusy(true);
    try {
      const body = {
        ...form,
        line_id: Number(form.line_id),
        vehicle_id: form.vehicle_id ? Number(form.vehicle_id) : null,
        board_stop_id: form.board_stop_id ? Number(form.board_stop_id) : null,
        alight_stop_id: form.alight_stop_id ? Number(form.alight_stop_id) : null,
        transfer_line_id: form.is_transfer && form.transfer_line_id ? Number(form.transfer_line_id) : null,
        transfer_stop_id: form.is_transfer && form.transfer_stop_id ? Number(form.transfer_stop_id) : null,
      };
      const d = await api('/api/reports', { method: 'POST', body });
      window.location.href = `/reports/${d.id}`;
    } catch (e) { setErr(e.message); } finally { setBusy(false); }
  }

  return (
    <div className="container" style={{ maxWidth: 860 }}>
      <Card title="遗失物品申报">
        <Err msg={err} />
        <div className="form-row-3">
          <label className="f"><span>物品类别 *</span>
            <select className="in" value={form.category} onChange={(e) => set('category', e.target.value)}>
              {meta.categories.map((x) => <option key={x}>{x}</option>)}
            </select>
          </label>
          <label className="f"><span>乘坐线路 *</span>
            <select className="in" value={form.line_id} onChange={(e) => set('line_id', e.target.value)}>
              <option value="">请选择</option>
              {meta.lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
            </select>
          </label>
          <label className="f"><span>车辆（记不清可不选）</span>
            <select className="in" value={form.vehicle_id} disabled={form.vehicle_unknown} onChange={(e) => set('vehicle_id', e.target.value)}>
              <option value="">请选择车牌</option>
              {lineVehicles.map((v) => <option key={v.id} value={v.id}>{v.plate_no}</option>)}
            </select>
          </label>
        </div>
        <label className="check">
          <input type="checkbox" checked={form.vehicle_unknown} onChange={(e) => set('vehicle_unknown', e.target.checked)} />
          记不清车牌号（如老人乘车）—— 由调度根据上下车站与时间排查候选车辆
        </label>
        <label className="f"><span>物品描述 *</span>
          <textarea className="in" value={form.description} onChange={(e) => set('description', e.target.value)} placeholder="如：黑色 iPhone 14，手机壳有卡通贴纸" />
        </label>
        <label className="f"><span>物品特征（便于认领核验）</span>
          <input className="in" value={form.features} onChange={(e) => set('features', e.target.value)} placeholder="颜色 / 品牌 / 特殊标记 / 内含物等" />
        </label>
        <div className="form-row">
          <label className="f"><span>上车站</span>
            <select className="in" value={form.board_stop_id} onChange={(e) => set('board_stop_id', e.target.value)}>
              <option value="">请选择</option>
              {(line?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
          <label className="f"><span>下车站</span>
            <select className="in" value={form.alight_stop_id} onChange={(e) => set('alight_stop_id', e.target.value)}>
              <option value="">请选择</option>
              {(line?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        </div>
        <div className="form-row-3">
          <label className="f"><span>上车时间</span>
            <input className="in" type="datetime-local" value={form.ride_start} onChange={(e) => set('ride_start', e.target.value)} />
          </label>
          <label className="f"><span>下车时间</span>
            <input className="in" type="datetime-local" value={form.ride_end} onChange={(e) => set('ride_end', e.target.value)} />
          </label>
          <label className="f"><span>座位位置</span>
            <input className="in" value={form.seat_position} onChange={(e) => set('seat_position', e.target.value)} placeholder="如：后排靠窗 / 前门附近" />
          </label>
        </div>
        <label className="check">
          <input type="checkbox" checked={form.is_transfer} onChange={(e) => set('is_transfer', e.target.checked)} />
          跨线路换乘后遗失（需同时排查换乘线路）
        </label>
        {form.is_transfer && (
          <div className="form-row">
            <label className="f"><span>换乘线路</span>
              <select className="in" value={form.transfer_line_id} onChange={(e) => set('transfer_line_id', e.target.value)}>
                <option value="">请选择</option>
                {meta.lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
              </select>
            </label>
            <label className="f"><span>换乘站</span>
              <select className="in" value={form.transfer_stop_id} onChange={(e) => set('transfer_stop_id', e.target.value)}>
                <option value="">请选择</option>
                {(tLine?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </label>
          </div>
        )}
        <div className="form-row-3">
          <label className="f"><span>联系人姓名 *</span>
            <input className="in" value={form.contact_name} onChange={(e) => set('contact_name', e.target.value)} />
          </label>
          <label className="f"><span>联系电话 *</span>
            <input className="in" value={form.contact_phone} onChange={(e) => set('contact_phone', e.target.value)} />
          </label>
          <label className="f"><span>身份证件号（选填，脱敏存储展示）</span>
            <input className="in" value={form.passenger_id_card} onChange={(e) => set('passenger_id_card', e.target.value)} />
          </label>
        </div>
        <div className="btn-row">
          <button className="btn" disabled={busy} onClick={submit}>{busy ? '提交中…' : '提交申报'}</button>
        </div>
      </Card>
    </div>
  );
}
