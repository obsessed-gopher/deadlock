import { notFound, redirect } from "next/navigation";

import { isId } from "@/api/client.ts";

// Target of the search form on the home page: /players?id=... -> /players/<id>.
export default async function PlayerSearch({ searchParams }: { searchParams: Promise<{ id?: string }> }) {
  const id = (await searchParams).id?.trim() ?? "";
  if (!isId(id)) notFound();
  redirect(`/players/${id}`);
}
