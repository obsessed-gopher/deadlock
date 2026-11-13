import { redirect } from "next/navigation";

import { api } from "@/api/client.ts";
import { attempt } from "@/lib/errors.ts";
import { parsePlayerRef } from "@/lib/playerRef.ts";

// Target of the search form: /players?id=... -> /players/<id>. Takes ids,
// SteamID2/3 and Steam profile links; a custom URL is resolved through the
// API (Steam). Problems go back to the form with a localized hint.
export default async function PlayerSearch({ searchParams }: { searchParams: Promise<{ id?: string }> }) {
  const ref = parsePlayerRef((await searchParams).id ?? "");
  if (!ref) redirect("/?error=badId");
  if (ref.kind === "id") redirect(`/players/${ref.id}`);

  const res = await attempt(() => api.resolveVanity(ref.name));
  if (res.ok) redirect(`/players/${res.data.accountId}`);
  redirect(res.kind === "notFound" ? "/?error=steamNotFound" : res.kind === "badRequest" ? "/?error=badId" : "/?error=steamDown");
}
