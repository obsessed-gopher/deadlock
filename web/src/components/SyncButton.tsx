"use client";

import { useActionState } from "react";

import type { SyncState } from "@/app/players/[id]/actions.ts";
import { useI18n } from "@/i18n/client.tsx";

export function SyncButton({ action }: { action: (prev: SyncState | null) => Promise<SyncState> }) {
  const { t } = useI18n();
  const [state, run, pending] = useActionState(action, null);
  return (
    <form action={run} className="row">
      <button type="submit" className="btn primary" disabled={pending}>
        {pending ? t("player.syncing") : t("player.sync")}
      </button>
      {state && (
        <span role="status" className={state.ok ? "muted small" : "tone-loss small"}>
          {state.message}
        </span>
      )}
    </form>
  );
}
