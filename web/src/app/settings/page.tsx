import type { Metadata } from "next";

import { LOCALES } from "@/i18n/core.ts";
import { getI18n } from "@/i18n/server.ts";

import { savePrefs } from "./actions.ts";

export async function generateMetadata(): Promise<Metadata> {
  return { title: (await getI18n()).t("settings.title") };
}

export default async function Settings({ searchParams }: { searchParams: Promise<{ saved?: string }> }) {
  const i18n = await getI18n();
  const { t } = i18n;
  const { saved } = await searchParams;
  return (
    <section className="stack" style={{ maxWidth: 560 }}>
      <h1>{t("settings.title")}</h1>
      {saved && (
        <p className="notice" role="status">
          {t("settings.saved")}
        </p>
      )}
      <form action={savePrefs} className="card stack" style={{ gap: 24 }}>
        <fieldset className="radio-list">
          <legend>{t("settings.language")}</legend>
          {LOCALES.map((l) => (
            <label key={l} lang={l}>
              <input type="radio" name="lang" value={l} defaultChecked={i18n.locale === l} />
              {t(`lang.${l}`)}
            </label>
          ))}
        </fieldset>
        <fieldset className="radio-list">
          <legend>{t("settings.names")}</legend>
          {LOCALES.map((l) => (
            <label key={l} lang={l}>
              <input type="radio" name="names" value={l} defaultChecked={i18n.names === l} />
              {t(`lang.${l}`)}
            </label>
          ))}
          <p className="faint small" style={{ margin: 0 }}>
            {t("settings.namesHint")}
          </p>
        </fieldset>
        <div>
          <button type="submit" className="btn primary">
            {t("settings.save")}
          </button>
        </div>
      </form>
    </section>
  );
}
