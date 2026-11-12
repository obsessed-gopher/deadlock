import { redirect } from "next/navigation";

import { isId } from "@/api/client.ts";

// Target of the search form: /players?id=... -> /players/<id>. A bad id goes
// back to the form with a localized hint instead of an error page.
export default async function PlayerSearch({ searchParams }: { searchParams: Promise<{ id?: string }> }) {
  const id = (await searchParams).id?.trim() ?? "";
  redirect(isId(id) ? `/players/${id}` : "/?error=badId");
}
