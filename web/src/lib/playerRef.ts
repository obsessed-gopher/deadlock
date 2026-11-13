// What a user may paste into the player search: an account id, a SteamID64,
// SteamID2/SteamID3 or a Steam profile link. Pure parsing; a custom URL
// (steamcommunity.com/id/<name>) still needs the API to become an id.

export type PlayerRef = { kind: "id"; id: string } | { kind: "vanity"; name: string };

const MAX_U32 = 0xffffffff;

/** Steam's custom URL alphabet (same check as the Go server). */
const VANITY_RE = /^[A-Za-z0-9_-]{2,64}$/;

const isDigits = (s: string) => /^\d{1,20}$/.test(s) && !/^0+$/.test(s);

/** null: not something that names a Steam account. */
export function parsePlayerRef(input: string): PlayerRef | null {
  const s = input.trim();
  if (isDigits(s)) return { kind: "id", id: s };

  // SteamID3: [U:1:1517505834]
  const id3 = /^\[?U:1:(\d{1,10})\]?$/i.exec(s);
  if (id3) return accountRef(Number(id3[1]));

  // SteamID2: STEAM_0:Y:Z -> Z * 2 + Y
  const id2 = /^STEAM_[0-5]:([01]):(\d{1,10})$/i.exec(s);
  if (id2) return accountRef(Number(id2[2]) * 2 + Number(id2[1]));

  return profileLink(s);
}

function accountRef(n: number): PlayerRef | null {
  return n > 0 && n <= MAX_U32 ? { kind: "id", id: String(n) } : null;
}

/** steamcommunity.com/profiles/<SteamID64> or /id/<name>, scheme optional. */
function profileLink(s: string): PlayerRef | null {
  let url: URL;
  try {
    url = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(s) ? s : `https://${s}`);
  } catch {
    return null;
  }
  const host = url.hostname.toLowerCase();
  if (host !== "steamcommunity.com" && !host.endsWith(".steamcommunity.com")) return null;
  const [kind, value] = url.pathname.split("/").filter(Boolean);
  if (!value) return null;
  if (kind === "profiles") return isDigits(value) ? { kind: "id", id: value } : null;
  if (kind === "id") {
    let name: string;
    try {
      name = decodeURIComponent(value);
    } catch {
      return null;
    }
    return VANITY_RE.test(name) ? { kind: "vanity", name } : null;
  }
  return null;
}
