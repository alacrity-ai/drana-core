export function DranaAmount({ microdrana, size = 20 }: { microdrana: number; size?: number }) {
  const drana = (microdrana / 1_000_000).toFixed(2);
  return (
    <span className="mono amber" style={{ fontSize: size, fontWeight: 500 }}>
      {drana} <span style={{ fontSize: size * 0.6 }}>DRANA</span>
    </span>
  );
}
