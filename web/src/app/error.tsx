"use client";

// Last resort for errors pages did not handle themselves (they render
// ErrorState for API failures). Localized through the layout's provider.
import Link from "next/link";

import { useI18n } from "@/i18n/client.tsx";

export default function ErrorPage({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const { t } = useI18n();
  return (
    <div className="state" role="alert">
      <span className="eyebrow">500</span>
      <h1>{t("error.internal.title")}</h1>
      <p>{t("error.internal.text")}</p>
      <div className="row">
        <button type="button" className="btn primary" onClick={reset}>
          {t("error.retry")}
        </button>
        <Link href="/" className="btn">
          {t("error.home")}
        </Link>
      </div>
    </div>
  );
}
