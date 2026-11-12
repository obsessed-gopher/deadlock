"use client";

// Locale for client components. Only translation is meant for the client:
// dates and numbers are formatted on the server and passed as text, so the
// server's and the browser's time zones can never disagree during hydration.
import { type ReactNode, createContext, useContext, useMemo } from "react";

import { type I18n, type Prefs, createI18n } from "./core.ts";

const I18nContext = createContext<I18n | null>(null);

export function I18nProvider({ prefs, children }: { prefs: Prefs; children: ReactNode }) {
  const value = useMemo(() => createI18n(prefs), [prefs]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  const v = useContext(I18nContext);
  if (!v) throw new Error("useI18n outside I18nProvider");
  return v;
}
