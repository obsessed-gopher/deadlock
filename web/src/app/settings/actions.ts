"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { LOCALE_COOKIE, NAMES_COOKIE, isLocale } from "@/i18n/core.ts";

const YEAR = 60 * 60 * 24 * 365;
const cookieOpts = { maxAge: YEAR, path: "/", sameSite: "lax" as const, httpOnly: true };

/** Settings form: interface language and names language. */
export async function savePrefs(form: FormData): Promise<void> {
  const c = await cookies();
  const lang = form.get("lang");
  const names = form.get("names");
  if (isLocale(lang)) c.set(LOCALE_COOKIE, lang, cookieOpts);
  if (isLocale(names)) c.set(NAMES_COOKIE, names, cookieOpts);
  redirect("/settings?saved=1");
}

/** Header switch: interface language only; stays on the same page. */
export async function setLocale(form: FormData): Promise<void> {
  const lang = form.get("lang");
  if (isLocale(lang)) (await cookies()).set(LOCALE_COOKIE, lang, cookieOpts);
  const back = form.get("back");
  // Only same-site paths: never redirect to an address taken from the form as is.
  redirect(typeof back === "string" && back.startsWith("/") && !back.startsWith("//") ? back : "/");
}
