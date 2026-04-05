export function LoadingDots() {
  return (
    <div style={{ textAlign: 'center', padding: '16px 0' }}>
      <span className="loading-dots">
        <span>.</span><span>.</span><span>.</span>
      </span>
      <style>{`
        .loading-dots span {
          font-size: 24px; color: var(--accent); opacity: 0.3;
          animation: dot-pulse 1.4s infinite;
        }
        .loading-dots span:nth-child(2) { animation-delay: 0.2s; }
        .loading-dots span:nth-child(3) { animation-delay: 0.4s; }
        @keyframes dot-pulse {
          0%, 80%, 100% { opacity: 0.3; }
          40% { opacity: 1; }
        }
      `}</style>
    </div>
  );
}
