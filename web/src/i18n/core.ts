// Locale handling without React or Next: preferences, translation and
// locale-aware formatting. Server (server.ts) and client (client.tsx) wrap it.
import { duration } from "../lib/format.ts";
import { type MessageKey, type Messages, en } from "./en.ts";
import { ru } from "./ru.ts";

export const LOCALES = ["en", "ru"] as const;
export type Locale = (typeof LOCALES)[number];

/** Language of hero and item names from the game files. */
export type NamesLang = Locale;

export interface Prefs {
  locale: Locale;
  names: NamesLang;
}

export const LOCALE_COOKIE = "lang";
export const NAMES_COOKIE = "names";

const dictionaries: Record<Locale, Messages> = { en, ru };

export function isLocale(v: unknown): v is Locale {
  return v === "en" || v === "ru";
}

/**
 * Interface language: the cookie if valid, else the browser's preference
 * (Accept-Language), else English. Names are English unless the user chose
 * Russian in settings, whatever the interface language.
 */
export function resolvePrefs(cookieLocale?: string, cookieNames?: string, acceptLanguage?: string | null): Prefs {
  let locale: Locale = "en";
  if (isLocale(cookieLocale)) {
    locale = cookieLocale;
  } else if (acceptLanguage) {
    const first = acceptLanguage
      .split(",")
      .map((part) => {
        const [tag = "", q] = part.trim().split(";q=");
        return { tag: tag.toLowerCase(), q: q === undefined ? 1 : Number(q) };
      })
      .filter((x) => x.tag && x.q > 0)
      .sort((a, b) => b.q - a.q)
      .find((x) => x.tag.startsWith("ru") || x.tag.startsWith("en"));
    if (first?.tag.startsWith("ru")) locale = "ru";
  }
  return { locale, names: isLocale(cookieNames) ? cookieNames : "en" };
}

export type Params = Record<string, string | number>;

/** Message with {placeholders} replaced; unknown placeholders stay visible. */
export function translate(locale: Locale, key: MessageKey, params?: Params): string {
  const text = dictionaries[locale][key];
  if (!params) return text;
  return text.replace(/\{(\w+)\}/g, (m, name: string) => (name in params ? String(params[name]) : m));
}

const intlLocale: Record<Locale, string> = { en: "en-GB", ru: "ru-RU" };

export interface I18n extends Prefs {
  t: (key: MessageKey, params?: Params) => string;
  /** Unix seconds (int64 arrives as a decimal string); 0 means never -> "—". */
  date: (unix: string | number) => string;
  num: (n: number) => string;
  /** part / whole as a percentage with one decimal; "—" when whole is 0. */
  pct: (part: number, whole: number) => string;
  /** Fixed decimals in the locale's notation. */
  dec: (n: number, digits: number) => string;
  duration: (seconds: number) => string;
  team: (team: number) => string;
  gameMode: (mode: number) => string;
  matchMode: (mode: number) => string;
  /** Hero role from the game data ("assassin"); unknown roles as is. */
  role: (type: string) => string;
  /** Display name of a hero or ability in the chosen names language. */
  name: (x: { name: string; nameRu: string; key: string }) => string;
}

function enumText(locale: Locale, prefix: "team" | "gameMode" | "matchMode", n: number): string {
  const key = `${prefix}.${n}`;
  return key in en ? translate(locale, key as MessageKey) : translate(locale, `${prefix}.n`, { n });
}

export function createI18n({ locale, names }: Prefs): I18n {
  const tag = intlLocale[locale];
  const dateFmt = new Intl.DateTimeFormat(tag, {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  const numFmt = new Intl.NumberFormat(tag);
  const pctFmt = new Intl.NumberFormat(tag, { style: "percent", minimumFractionDigits: 1, maximumFractionDigits: 1 });
  return {
    locale,
    names,
    t: (key, params) => translate(locale, key, params),
    date: (unix) => {
      const n = Number(unix);
      return n > 0 ? dateFmt.format(new Date(n * 1000)) : "—";
    },
    num: (n) => numFmt.format(n),
    pct: (part, whole) => (whole > 0 ? pctFmt.format(part / whole) : "—"),
    dec: (n, digits) =>
      new Intl.NumberFormat(tag, { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(n),
    duration,
    team: (n) => enumText(locale, "team", n),
    gameMode: (n) => enumText(locale, "gameMode", n),
    matchMode: (n) => enumText(locale, "matchMode", n),
    role: (type) => (`role.${type}` in en ? translate(locale, `role.${type}` as MessageKey) : type),
    name: (x) => (names === "ru" ? x.nameRu || x.name || x.key : x.name || x.key),
  };
}

/** Placeholders of a message, for checking that translations keep them. */
export function placeholders(text: string): string[] {
  return [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1] ?? "").sort();
}

export { dictionaries };
