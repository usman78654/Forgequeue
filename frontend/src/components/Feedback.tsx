import type {NoticeMessage, StatusValue} from '../types';

interface NoticeProps {
  notice: NoticeMessage | null;
  onDismiss: () => void;
}

export function Notice({notice, onDismiss}: NoticeProps) {
  if (!notice) return null;
  const role = notice.kind === 'error' ? 'alert' : 'status';
  return (
    <div
      className={`notice ${notice.kind || 'info'}`}
      role={role}
      aria-live={role === 'alert' ? 'assertive' : 'polite'}
    >
      <span>{notice.message}</span>
      <button className="icon-button" onClick={onDismiss} aria-label="Dismiss notification">
        ×
      </button>
    </div>
  );
}

export function LoadingState({label = 'Loading'}: {label?: string}) {
  return (
    <div className="state-card" role="status">
      <span className="spinner" aria-hidden="true" />
      <span>{label}…</span>
    </div>
  );
}

export function EmptyState({title, message}: {title: string; message: string}) {
  return (
    <div className="state-card empty">
      <strong>{title}</strong>
      <span>{message}</span>
    </div>
  );
}

export function StatusBadge({status}: {status: StatusValue}) {
  return <span className={`status ${status}`}>{status}</span>;
}
