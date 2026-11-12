import Link from "next/link";
import { Suspense } from "react";

import type { I18n } from "@/i18n/core.ts";

import { LangSwitch } from "./LangSwitch.tsx";

export function Logo() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 3v18M3 12h18" />
    </svg>
  );
}

export function Header({ i18n }: { i18n: I18n }) {
  const { t } = i18n;
  return (
    <header className="header">
      <div className="container header-inner">
        <Link href="/" className="brand">
          <Logo />
          {t("app.name")}
        </Link>
        <nav className="nav" aria-label={t("app.title")}>
          <Link href="/">{t("nav.players")}</Link>
          <Link href="/settings">{t("nav.settings")}</Link>
        </nav>
        {/* useSearchParams needs a Suspense boundary. */}
        <Suspense>
          <LangSwitch />
        </Suspense>
      </div>
    </header>
  );
}
