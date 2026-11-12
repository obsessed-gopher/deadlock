"use client";

// Shown when the Go API is down or returns an unexpected error.
export default function ErrorPage({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <div className="card error" style={{ maxWidth: 560 }}>
      <h1>Ошибка</h1>
      <p className="muted">
        Не удалось получить данные от API. Проверь, что сервер запущен (<code>docker compose ps</code>).
      </p>
      {error.digest && <p className="muted small">digest: {error.digest}</p>}
      <button type="button" onClick={reset}>
        Повторить
      </button>
    </div>
  );
}
