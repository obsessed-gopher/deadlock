// Steam avatar, round. Without a picture (profile not fetched yet, mock
// data) it is the first letter of the name on a neutral disc, same size.
// Decorative: the name is always next to it as text.
export function Avatar({ src, name, size = 32 }: { src?: string; name: string; size?: number }) {
  if (src) {
    return (
      <img
        className="avatar"
        src={src}
        alt=""
        width={size}
        height={size}
        loading="lazy"
        decoding="async"
        referrerPolicy="no-referrer"
      />
    );
  }
  const letter = [...name.trim()][0]?.toUpperCase() ?? "?";
  return (
    <span className="avatar avatar-empty" style={{ width: size, height: size, fontSize: Math.round(size * 0.45) }} aria-hidden="true">
      {letter}
    </span>
  );
}
