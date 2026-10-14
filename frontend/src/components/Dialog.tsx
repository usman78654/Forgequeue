import {useEffect, useId, useRef, type MouseEvent, type ReactNode} from 'react';

/* eslint-disable jsx-a11y/no-static-element-interactions -- Backdrop dismissal is an optional pointer shortcut; the dialog also has a named close button and Escape support. */

interface DialogProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  actions?: ReactNode;
}

export function Dialog({title, children, onClose, actions}: DialogProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLElement | null>(null);
  const previousFocus = useRef<Element | null>(document.activeElement);

  useEffect(() => {
    const dialog = dialogRef.current;
    const restoreFocus = previousFocus.current;
    dialog?.querySelector<HTMLElement>('button, input, select, textarea')?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      // Only the topmost dialog owns keyboard dismissal and focus trapping.
      // Artifact confirmations can be opened inside a job-inspection dialog.
      const dialogs = document.querySelectorAll('[role="dialog"][aria-modal="true"]');
      if (dialogs[dialogs.length - 1] !== dialog) return;
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab' || !dialog) return;
      const focusable = Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [href]',
        ),
      );
      if (!focusable.length) return;
      const first = focusable.at(0);
      const last = focusable.at(-1);
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      if (restoreFocus instanceof HTMLElement) restoreFocus.focus();
    };
  }, [onClose]);

  return (
    <div
      className="modal"
      onMouseDown={(event: MouseEvent<HTMLDivElement>) =>
        event.target === event.currentTarget && onClose()
      }
    >
      <section
        ref={dialogRef}
        className="card form dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <div className="dialog-heading">
          <h2 id={titleId}>{title}</h2>
          <button className="icon-button" onClick={onClose} aria-label="Close dialog">
            ×
          </button>
        </div>
        {children}
        {actions && <div className="dialog-actions">{actions}</div>}
      </section>
    </div>
  );
}
