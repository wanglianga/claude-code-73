'use client';

import { useEffect } from 'react';
import { getUser } from '../lib/api';

export default function Home() {
  useEffect(() => {
    window.location.href = getUser() ? '/dashboard' : '/login';
  }, []);
  return <div className="empty">正在跳转…</div>;
}
