"use server";

import { revalidatePath } from "next/cache";

import { api, isId } from "@/api/client.ts";
import { getI18n } from "@/i18n/server.ts";
import { errorKind } from "@/lib/errors.ts";

export interface SyncState {
  message: string;
  ok: boolean;
}

/** Sync button: localized result text; failures never throw to the page. */
export async function syncPlayer(accountId: string, _prev: SyncState | null): Promise<SyncState> {
  const { t } = await getI18n();
  if (!isId(accountId)) return { ok: false, message: t("error.badId") };
  try {
    const r = await api.sync(accountId);
    // All profile tabs (progress, heroes, matches) live under this path.
    revalidatePath(`/players/${accountId}`, "layout");
    return { ok: true, message: t("player.syncDone", { new: r.newMatches, queued: r.queuedMeta }) };
  } catch (err) {
    const kind = errorKind(err);
    return { ok: false, message: t("error.syncFailed", { reason: t(`error.${kind}.text`) }) };
  }
}
