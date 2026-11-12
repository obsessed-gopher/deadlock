"use client";

import { useActionState } from "react";

import type { SyncState } from "@/app/players/[id]/actions.ts";

export function SyncButton({ action }: { action: (prev: SyncState | null) => Promise<SyncState> }) {
  const [state, run, pending] = useActionState(action, null);
  return (
    <form action={run} className="row">
      <button type="submit" className="primary" disabled={pending}>
        {pending ? "Синхронизация…" : "Синхронизировать"}
      </button>
      {state && <span className={state.ok ? "muted small" : "loss small"}>{state.message}</span>}
    </form>
  );
}
