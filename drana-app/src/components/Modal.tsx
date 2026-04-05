import { useEffect, type ReactNode } from 'react';

export function Modal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-panel" onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h2 style={{ fontSize: 18, fontWeight: 600 }}>{title}</h2>
          <button onClick={onClose} style={{ color: 'var(--text-muted)', fontSize: 20 }}>&times;</button>
        </div>
        {children}
      </div>
      <style>{`
        .modal-overlay {
          position: fixed; inset: 0; background: rgba(10,10,15,0.8);
          backdrop-filter: blur(4px); display: flex; align-items: center;
          justify-content: center; z-index: 100; animation: fadeIn 150ms;
        }
        .modal-panel {
          background: var(--bg-surface); border: 1px solid var(--border);
          max-width: 420px; width: 100%; padding: 32px;
          animation: slideUp 150ms;
        }
        .btn-primary {
          background: var(--accent); color: var(--bg-primary); font-weight: 600;
          font-size: 14px; width: 100%; height: 44px; border: none; cursor: pointer;
          transition: background var(--transition);
        }
        .btn-primary:hover { background: var(--accent-hover); }
        .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
        @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
        @keyframes slideUp { from { transform: translateY(8px); opacity: 0; } to { transform: translateY(0); opacity: 1; } }
      `}</style>
    </div>
  );
}
