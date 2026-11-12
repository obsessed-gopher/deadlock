import Link from "next/link";

import { getI18n } from "@/i18n/server.ts";

// Reads SYNC_ACCOUNTS and cookies at request time.
export const dynamic = "force-dynamic";

/** Accounts the server syncs on its own (same variable as the Go server). */
function trackedAccounts(): string[] {
  return (process.env.SYNC_ACCOUNTS ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter((s) => /^\d+$/.test(s));
}

export default async function Home({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const i18n = await getI18n();
  const { t } = i18n;
  const { error } = await searchParams;
  const tracked = trackedAccounts();
  return (
    <>
      <section className="hero">
        <span className="eyebrow">{t("app.title")}</span>
        <h1>{t("home.title")}</h1>
        <p>{t("home.subtitle")}</p>
        {/* Plain GET form: works without client JavaScript. */}
        <form action="/players" method="get" className="stack">
          <label className="field big">
            <input
              name="id"
              required
              inputMode="numeric"
              autoComplete="off"
              placeholder="1517505834"
              aria-label={t("home.inputLabel")}
              aria-describedby={error ? "search-error" : undefined}
            />
            <button type="submit" className="btn primary">
              {t("home.open")}
            </button>
          </label>
          {error === "badId" && (
            <p id="search-error" className="notice error" role="alert">
              {t("error.badId")}
            </p>
          )}
        </form>
      </section>
      {tracked.length > 0 && (
        <section aria-labelledby="tracked">
          <h2 id="tracked">{t("home.tracked")}</h2>
          <div className="row">
            {tracked.map((id) => (
              <Link key={id} href={`/players/${id}`} className="btn small mono">
                {id}
              </Link>
            ))}
          </div>
        </section>
      )}
    </>
  );
}
