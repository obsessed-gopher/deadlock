"use client";

// Replaces the root layout when the layout itself fails, so neither the
// locale nor the styles are available: both languages, inline styles.
export default function GlobalError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <html lang="en">
      <body style={{ margin: 0, background: "#000", color: "#ededed", font: "15px/1.5 system-ui, sans-serif" }}>
        <main style={{ maxWidth: 560, margin: "15vh auto", padding: "0 16px" }}>
          <h1 style={{ fontSize: 32, margin: "0 0 8px" }}>Something went wrong</h1>
          <p lang="ru" style={{ margin: "0 0 24px", color: "#a1a1a1" }}>
            Что-то пошло не так. The page could not be rendered; details are in the server log.
          </p>
          <button
            type="button"
            onClick={reset}
            style={{ font: "inherit", padding: "8px 16px", borderRadius: 999, border: 0, background: "#ededed", color: "#000" }}
          >
            Try again / Повторить
          </button>
        </main>
      </body>
    </html>
  );
}
