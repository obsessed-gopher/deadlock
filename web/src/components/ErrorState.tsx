import Link from "next/link";

import type { I18n } from "@/i18n/core.ts";
import type { ErrorKind } from "@/lib/errors.ts";

/**
 * Localized error page body. Pages render it for failed API calls instead of
 * throwing, so a down or failing API shows an explanation, not a 500.
 */
export function ErrorState({ i18n, kind, retryHref }: { i18n: I18n; kind: ErrorKind; retryHref?: string }) {
  const { t } = i18n;
  return (
    <div className="state" role={kind === "notFound" ? undefined : "alert"}>
      <span className="eyebrow">{kind === "notFound" ? "404" : kind === "badRequest" ? "400" : kind === "unavailable" ? "503" : "500"}</span>
      <h1>{t(`error.${kind}.title`)}</h1>
      <p>{t(`error.${kind}.text`)}</p>
      <div className="row">
        {retryHref && kind !== "notFound" && kind !== "badRequest" && (
          <Link href={retryHref} className="btn primary">
            {t("error.retry")}
          </Link>
        )}
        <Link href="/" className="btn">
          {t("error.home")}
        </Link>
      </div>
    </div>
  );
}
