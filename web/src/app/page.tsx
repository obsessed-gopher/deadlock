import Link from "next/link";

// Reads SYNC_ACCOUNTS at request time, not at build time.
export const dynamic = "force-dynamic";

/** Accounts the server syncs on its own (same variable as the Go server). */
function trackedAccounts(): string[] {
  return (process.env.SYNC_ACCOUNTS ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter((s) => /^\d+$/.test(s));
}

export default function Home() {
  const tracked = trackedAccounts();
  return (
    <div className="card" style={{ maxWidth: 560 }}>
      <h1>Найти игрока</h1>
      <p className="muted">Account id или SteamID64.</p>
      {/* Plain GET form: works without client JavaScript. */}
      <form action="/players" method="get" className="row">
        <input
          name="id"
          required
          inputMode="numeric"
          pattern="\d+"
          placeholder="1517505834"
          aria-label="Account id"
          style={{ flex: 1, minWidth: 200 }}
        />
        <button type="submit" className="primary">
          Открыть
        </button>
      </form>
      {tracked.length > 0 && (
        <>
          <h2>Отслеживаемые</h2>
          <div className="row">
            {tracked.map((id) => (
              <Link key={id} href={`/players/${id}`} className="button">
                {id}
              </Link>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
