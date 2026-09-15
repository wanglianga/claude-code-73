'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { api, getUser } from '../../../lib/api';
import { Card, Err, Photos, Uploader } from '../../../components/ui';
import { toLocalInput } from '../../../lib/util';

export default function NewItem() {
  const [user, setUser] = useState(null);
  const [meta, setMeta] = useState(null);
  const [pendingId, setPendingId] = useState(null);
  const [pendingValuable, setPendingValuable] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');
  const [form, setForm] = useState({
    category: '其他', description: '', features: '', photos: [],
    found_line_id: '', found_vehicle_id: '', found_stop_id: '',
    found_at: toLocalInput(new Date()), handed_by_role: 'driver', handed_by_name: '',
    storage_cabinet: '', value_level: '普通', special_type: '',
  });
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    const u = getUser();
    if (!u) { window.location.href = '/login'; return; }
    setUser(u);
    api('/api/meta').then(setMeta).catch((e) => setErr(e.message));
    const q = new URLSearchParams(window.location.search);
    const id = q.get('id');
    if (id) {
      setPendingId(id);
      api(`/api/items/${id}`).then((d) => {
        const it = d.item;
        setForm((f) => ({
          ...f,
          category: it.category, description: it.description, features: it.features || '',
          found_line_id: it.found_line_id || '', found_vehicle_id: it.found_vehicle_id || '',
          found_stop_id: it.found_stop_id || '',
          found_at: it.found_at ? it.found_at.slice(0, 16) : f.found_at,
          handed_by_role: it.handed_by_role || 'driver', handed_by_name: it.handed_by_name || '',
        }));
        setPendingValuable(['手机', '钱包'].includes(it.category) || it.value_level === '贵重');
      }).catch((e) => setErr(e.message));
    }
  }, []);

  if (!user || !meta) return <div className="container"><div className="empty">加载中…</div></div>;

  const valuableSelected = form.value_level === '贵重' || ['手机', '钱包'].includes(form.category);

  if (pendingValuable) {
    return (
      <div className="container" style={{ maxWidth: 720 }}>
        <Card title="该物品须双人入柜">
          <div className="alert alert-warn">
            司机上交的是<b>手机 / 钱包 / 贵重物品</b>，不得由站务单人登记入库。须由
            <b>站务与安保共同拍照、封袋、入柜并记录柜号</b>，再由安保会签确认。
          </div>
          <Link className="btn" href="/valuable/new">前往办理双人入柜</Link>
        </Card>
      </div>
    );
  }

  const line = meta.lines.find((l) => String(l.id) === String(form.found_line_id));
  const lineVehicles = meta.vehicles.filter((v) => String(v.line_id) === String(form.found_line_id));

  // 特殊类别提示（后端会按规则强制）
  const specialHint = {
    银行卡: '将触发：挂失提醒告知，保管期 15 天，逾期剪角销毁',
    药品: '将触发：冷藏柜保管，保管期 7 天，逾期规范销毁',
    儿童物品: '将触发：儿童证件规则，优先联系监护人，保管期 90 天',
    危险品: '将触发：不入柜，安保暂存 + 紧急报警，3 日内移交公安',
    证件: '将触发：隐私脱敏展示，保管期 90 天，逾期移交公安',
  }[form.category];
  async function submit() {
    setErr(''); setOk('');
    try {
      const body = {
        ...form,
        found_line_id: form.found_line_id ? Number(form.found_line_id) : null,
        found_vehicle_id: form.found_vehicle_id ? Number(form.found_vehicle_id) : null,
        found_stop_id: form.found_stop_id ? Number(form.found_stop_id) : null,
      };
      const d = pendingId
        ? await api(`/api/items/${pendingId}/register`, { method: 'POST', body })
        : await api('/api/items', { method: 'POST', body });
      window.location.href = `/items/${d.id}`;
    } catch (e) { setErr(e.message); }
  }

  return (
    <div className="container" style={{ maxWidth: 860 }}>
      <Card title={pendingId ? `完成登记入库（待登记 #${pendingId}）` : '站务登记入库'}>
        {ok && <div className="alert alert-ok">{ok}</div>}
        <Err msg={err} />
        {valuableSelected && (
          <div className="alert alert-danger">
            <b>手机 / 钱包 / 贵重物品不得单人登记入库。</b>
            须由站务与安保共同拍照、封袋、入柜并会签。<Link href="/valuable/new">前往「贵重物品双人入柜」→</Link>
          </div>
        )}
        {specialHint && <div className="alert alert-warn">{specialHint}</div>}
        <div className="form-row-3">
          <label className="f"><span>物品类别 *</span>
            <select className="in" value={form.category} onChange={(e) => set('category', e.target.value)}>
              {meta.categories.map((x) => <option key={x}>{x}</option>)}
            </select>
          </label>
          <label className="f"><span>贵重程度</span>
            <select className="in" value={form.value_level} onChange={(e) => set('value_level', e.target.value)}>
              <option>普通</option><option>贵重</option>
            </select>
          </label>
          <label className="f"><span>存放柜 *（特殊类别自动指定）</span>
            <select className="in" value={form.storage_cabinet} onChange={(e) => set('storage_cabinet', e.target.value)}>
              <option value="">按规则自动 / 请选择</option>
              {meta.cabinets.map((c) => <option key={c}>{c}</option>)}
            </select>
          </label>
        </div>
        <label className="f"><span>物品描述 *</span>
          <textarea className="in" value={form.description} onChange={(e) => set('description', e.target.value)} />
        </label>
        <label className="f"><span>物品特征</span>
          <input className="in" value={form.features} onChange={(e) => set('features', e.target.value)} />
        </label>
        <div className="f">
          <span className="small muted">物品照片（登记留档）</span>
          <div className="flex mt">
            <Photos urls={form.photos} />
            <Uploader label="+ 添加照片" onUploaded={(url) => set('photos', [...form.photos, url])} />
          </div>
        </div>
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
          <label className="f"><span>或发现站点</span>
            <select className="in" value={form.found_stop_id} onChange={(e) => set('found_stop_id', e.target.value)}>
              <option value="">请选择</option>
              {(line?.stops || []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        </div>
        <div className="form-row-3">
          <label className="f"><span>发现时间</span>
            <input className="in" type="datetime-local" value={form.found_at} onChange={(e) => set('found_at', e.target.value)} />
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
        <div className="btn-row">
          <button className="btn" disabled={valuableSelected} onClick={submit}>登记入库</button>
        </div>
        <div className="small muted mt">保管期限与保管规则由系统按物品类别自动生成；贵重物品 / 危险品将自动联动安保报警。</div>
      </Card>
    </div>
  );
}
