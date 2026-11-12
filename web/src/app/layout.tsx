import type { Metadata } from "next";
import Link from "next/link";
import type { ReactNode } from "react";

import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Deadlock: аналитика", template: "%s · Deadlock" },
  description: "Личная аналитика матчей Deadlock",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="ru">
      <body>
        <header className="header">
          <div className="header-inner">
            <Link href="/" className="brand">
              Deadlock <span>·</span> аналитика
            </Link>
          </div>
        </header>
        <main className="main">{children}</main>
      </body>
    </html>
  );
}
