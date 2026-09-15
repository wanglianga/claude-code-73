'use client';

import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { Card, Err, Photos, Uploader } from '../../../components/ui';
import { toLocalInput } from '../../../lib/util';

export default function ValuableNewPage() {
  const [user, setUser] = useState(null);
  const [meta, setMeta] = useState(null);
  const [pending, setPending] = useState([]);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState('pending'); // pending=选择司机已上交物品；direct=站务直接补录
  const [form, setForm] = useState({
    item_id: '',
    cabinet_no: '', seal_no: '', photos: [], notes: '',
    category: '手机', description: '', features: '',
    found_line_id: '', found_vehicle_id: '', found_at: toLocalInput(new Date()),
    handed_by_role: 'driver', handed_by_name: '',
  });
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    if (!['station', 'admin'].includes(u.role)) { window.location.href = '/valuable'; return; }
    setUser(u);
    api('/api/meta').then(setMeta).catch((e) => setErr(e.message));
    api('/api/items?status=pending_register').then((d) => {
      const v = (d.items || []).filter((it) => ['手机', '钱包'].includes(it.category) || it.value_level === '贵重');
      setPending(v);
    }).catch(() => {});
  }, []);

  if (!user || !meta) return <div className="container"><div className="empty">加载中…</div></div>;
  const line = meta.lines.find((l) => String(l.id) === String(form.found_line_id));
  const lineVehicles = meta.vehicles.filter((v) => String(v.line_id) === String(form.found_line_id));

  async function submit() {
    setErr('');
    if (!form.cabinet_no || !form.seal_no) { setErr('柜号与封袋编号为必填项'); return; }
    if (form.photos.length === 0) { setErr('请上传站务与安保共同拍照（至少 1 张）'); return; }
    if (mode === 'direct' && !form.description.trim()) { setErr('物品描述为必填项'); return; }
    setBusy(true);
    try {
      const body = mode === 'pending'
        ? { item_id: Number(form.item_id), cabinet_no: form.cabinet_no, seal_no: form.seal_no, station_photos: form.photos, notes: form.notes }
        : {
            item_id: 0, cabinet_no: form.cabinet_no, seal_no: form.seal_no, station_photos: form.photos, notes: form.notes,
            category: form.category, description: form.description, features: form.features,
            found_line_id: form.found_line_id ? Number(form.found_line_id) : null,
            found_vehicle_id: form.found_vehicle_id ? Number(form.found_vehicle_id) : null,
            found_at: form.found_at, handed_by_role: form.handed_by_role, handed_by_name: form.handed_by_name,
          };
      const d = await api('/api/valuable/intakes', { method: 'POST', body });
      window.location.href = `/items/${d.item_id}`;
    } catch (e) { setErr(e.message); } finally { setBusy(false); }
  }

  return (
    <div className="container" style={{ maxWidth: 860 }}>
      <Card title="贵重物品双人入柜 · 站务发起（安保会签）">
        <div className="alert alert-warn">
          流程：司机上交手机/钱包 → <b>站务与安保共同拍照</b> → 填写封袋编号与柜号 → 封袋入柜 → 安保登录核对会签。
          封袋破损不得入柜；站务与安保会签不能为同一人。
        </div>
        <Err msg={err} />

        <label className="f"><span>物品来源</span>
          <select className="in" value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="pending">司机/保洁已上交的待登记贵重物品</option>
            <option value="direct">站务直接补录物品信息</option>
          </select>
        </label>

        {mode === 'pending' ? (
          <label className="f"><span>选择待入柜物品 *</span>
            <select className="in" value={form.item_id} onChange={(e) => set('item_id', e.target.value)}>
              <option value="">请选择（仅手机/钱包/贵重物品）</option>
              {pending.map((it) => <option key={it.id} value={it.id}>{it.item_no} · {it.category} · {it.description}</option>)}
            </select>
          </label>
        ) : (
          <>
            <div className="form-row-3">
              <label className="f"><span>物品类别 *</span>
                <select className="in" value={form.category} onChange={(e) => set('category', e.target.value)}>
                  <option>手机</option><option>钱包</option><option>其他</option>
                </select>
              </label>
              <label className="f"><span>上交人</span>
                <select className="in" value={form.handed_by_role} onChange={(e) => set('handed_by_role', e.target.value)}>
                  <option value="driver">司机</option><option value="cleaner">保洁</option>
                </select>
              </label>
              <label className="f"><span>上交人姓名</span>
                <input className="in" value={form.handed_by_name} onChange={(e) => set('handed_by_name', e.target.value)} />
              </label>
            </div>
            <label className="f"><span>物品描述 *</span>
              <textarea className="in" value={form.description} onChange={(e) => set('description', e.target.value)} placeholder="如：棕色皮质钱包，内有现金约500元" />
            </label>
            <label className="f"><span>物品特征</span>
              <input className="in" value={form.features} onChange={(e) => set('features', e.target.value)} />
            </label>
            <div className="form-row-3">
              <label className="f"><span>发现线路</span>
                <select className="in" value={form.found_line_id} onChange={(e) => set('found_line_id', e.target.value)}>
                  <option value="">请选择</option>
                  {meta.lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
                </select>
              </label>
              <label className="f"><span>发现车辆</span>
                <select className="in" value={form.found_vehicle_id} onChange={(e) => set('found_vehicle_id', e.target.value)}>
                  <option value="">请选择</option>
                  {lineVehicles.map((v) => <option key={v.id} value={v.id}>{v.plate_no}</option>)}
                </select>
              </label>
              <label className="f"><span>发现时间</span>
                <input className="in" type="datetime-local" value={form.found_at} onChange={(e) => set('found_at', e.target.value)} />
              </label>
            </div>
          </>
        )}

        <div className="form-row">
          <label className="f"><span>保险柜柜号 *</span>
            <select className="in" value={form.cabinet_no} onChange={(e) => set('cabinet_no', e.target.value)}>
              <option value="">请选择</option>
              {meta.vault_cabinets.map((c) => <option key={c}>{c}</option>)}
            </select>
          </label>
          <label className="f"><span>封袋编号 *（一次性封签）</span>
            <input className="in" value={form.seal_no} onChange={(e) => set('seal_no', e.target.value)} placeholder="如：FB-1008" />
          </label>
        </div>

        <div className="f">
          <span className="small muted">站务与安保共同拍照 *（物品+封袋+柜号同框，至少 1 张）</span>
          <div className="flex mt">
            <Photos urls={form.photos} />
            <Uploader label="+ 添加共同拍照" onUploaded={(u) => set('photos', [...form.photos, u])} />
          </div>
        </div>
        <label className="f"><span>备注</span>
          <input className="in" value={form.notes} onChange={(e) => set('notes', e.target.value)} />
        </label>

        <div className="btn-row">
          <button className="btn" disabled={busy} onClick={submit}>{busy ? '提交中…' : '发起入柜（通知安保会签）'}</button>
        </div>
        <div className="small muted mt">提交后物品立即入柜上锁、责任人为发起站务；安保会签前显示「待会签」，会签后双人入柜完成。</div>
      </Card>
    </div>
  );
}
