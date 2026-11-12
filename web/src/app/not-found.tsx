import Link from "next/link";

export default function NotFound() {
  return (
    <div className="card" style={{ maxWidth: 560 }}>
      <h1>Не найдено</h1>
      <p className="muted">Такого игрока или матча нет в базе.</p>
      <Link href="/" className="button">
        На главную
      </Link>
    </div>
  );
}
