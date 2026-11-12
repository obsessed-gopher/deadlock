import type { Metadata } from "next";
import type { ReactNode } from "react";

import { Header } from "@/components/Header.tsx";
import { I18nProvider } from "@/i18n/client.tsx";
import { createI18n } from "@/i18n/core.ts";
import { getI18n, getPrefs } from "@/i18n/server.ts";

import "./globals.css";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getI18n();
  return { title: { default: t("app.title"), template: `%s · ${t("app.name")}` }, description: t("app.description") };
}

export default async function RootLayout({ children }: { children: ReactNode }) {
  const prefs = await getPrefs();
  const i18n = createI18n(prefs);
  return (
    <html lang={prefs.locale}>
      <body>
        <I18nProvider prefs={prefs}>
          <Header i18n={i18n} />
          <main className="container main">{children}</main>
        </I18nProvider>
      </body>
    </html>
  );
}
