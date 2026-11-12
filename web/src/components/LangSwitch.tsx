"use client";

import { usePathname, useSearchParams } from "next/navigation";

import { setLocale } from "@/app/settings/actions.ts";
import { useI18n } from "@/i18n/client.tsx";
import { LOCALES } from "@/i18n/core.ts";

/** RU / EN switch in the header; a plain form, so it works without JS too. */
export function LangSwitch() {
  const { locale, t } = useI18n();
  const path = usePathname();
  const search = useSearchParams().toString();
  return (
    <form action={setLocale} className="segmented" aria-label={t("nav.language")}>
      <input type="hidden" name="back" value={search ? `${path}?${search}` : path} />
      {LOCALES.map((l) => (
        <button
          key={l}
          type="submit"
          name="lang"
          value={l}
          aria-pressed={l === locale}
          lang={l}
          title={t(`lang.${l}`)}
        >
          {l.toUpperCase()}
        </button>
      ))}
    </form>
  );
}
