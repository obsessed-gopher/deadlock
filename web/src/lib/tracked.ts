// Accounts the server syncs on its own (SYNC_ACCOUNTS, the same variable as
// the Go server). The first one is "me": the owner the comparison page and
// profile shortcuts are built around. Server-side only.
const STEAM_ID64_BASE = 76561197960265728n;
const MAX_U32 = 0xffffffffn;

/** Account id of an account id or SteamID64 string; null for anything else. */
export function toAccountId(s: string): string | null {
  if (!/^\d{1,20}$/.test(s)) return null;
  const v = BigInt(s);
  if (v === 0n) return null;
  if (v <= MAX_U32) return String(v);
  if (v > STEAM_ID64_BASE && v - STEAM_ID64_BASE <= MAX_U32) return String(v - STEAM_ID64_BASE);
  return null;
}

export function parseAccounts(env: string | undefined): string[] {
  const ids = (env ?? "").split(",").map((s) => toAccountId(s.trim()));
  return [...new Set(ids.filter((s): s is string => s !== null))];
}

export function trackedAccounts(): string[] {
  return parseAccounts(process.env.SYNC_ACCOUNTS);
}

/** The owner's account id, or null when the server tracks nobody. */
export function ownerAccount(): string | null {
  return trackedAccounts()[0] ?? null;
}
