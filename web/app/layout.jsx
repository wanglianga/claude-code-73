import './globals.css';
import { Nav } from '../components/ui';

export const metadata = {
  title: '城市公交失物招领与监控调阅平台',
  description: '公交失物申报、查找、调阅、上交、认领、逾期处置一体化平台',
};

export default function RootLayout({ children }) {
  return (
    <html lang="zh-CN">
      <body>
        <Nav />
        {children}
      </body>
    </html>
  );
}
