import Link from "next/link";

import { getI18n } from "@/i18n/server.ts";
import { trackedAccounts } from "@/lib/tracked.ts";

// Reads SYNC_ACCOUNTS and cookies at request time.
export const dynamic = "force-dynamic";

/** ?error= values set by the search redirect (app/players/page.tsx). */
const SEARCH_ERRORS = ["badId", "steamNotFound", "steamDown"] as const;
type SearchError = (typeof SEARCH_ERRORS)[number];

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
              autoComplete="off"
              placeholder={t("home.placeholder")}
              aria-label={t("home.inputLabel")}
              aria-describedby={error && SEARCH_ERRORS.includes(error as SearchError) ? "search-error" : undefined}
            />
            <button type="submit" className="btn primary">
              {t("home.open")}
            </button>
          </label>
          {error && SEARCH_ERRORS.includes(error as SearchError) && (
            <p id="search-error" className="notice error" role="alert">
              {t(`error.${error as SearchError}`)}
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
