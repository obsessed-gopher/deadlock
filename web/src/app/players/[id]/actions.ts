"use server";

import { revalidatePath } from "next/cache";

import { api, isId } from "@/api/client.ts";

export interface SyncState {
  message: string;
  ok: boolean;
}

export async function syncPlayer(accountId: string, _prev: SyncState | null): Promise<SyncState> {
  if (!isId(accountId)) return { ok: false, message: "Некорректный id" };
  try {
    const r = await api.sync(accountId);
    revalidatePath(`/players/${accountId}`);
    return { ok: true, message: `Новых матчей: ${r.newMatches}, в очереди метаданных: ${r.queuedMeta}` };
  } catch (err) {
    return { ok: false, message: `Не удалось синхронизировать: ${err instanceof Error ? err.message : String(err)}` };
  }
}
