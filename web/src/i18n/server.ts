// Locale of the current request, for server components and actions.
import { cookies, headers } from "next/headers";

import { type I18n, LOCALE_COOKIE, NAMES_COOKIE, type Prefs, createI18n, resolvePrefs } from "./core.ts";

export async function getPrefs(): Promise<Prefs> {
  const [c, h] = await Promise.all([cookies(), headers()]);
  return resolvePrefs(c.get(LOCALE_COOKIE)?.value, c.get(NAMES_COOKIE)?.value, h.get("accept-language"));
}

export async function getI18n(): Promise<I18n> {
  return createI18n(await getPrefs());
}
