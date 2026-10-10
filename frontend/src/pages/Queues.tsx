import {useCallback, useEffect, useState} from 'react';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {errorMessage, type Page, type PageProps, type QueueControl} from '../types';

interface QueueChange {
  queue: QueueControl;
  paused: boolean;
  maxConcurrency: string;
  kind: 'availability' | 'concurrency';
}

export function Queues({api, notify, principal}: PageProps) {
  const [items, setItems] = useState<QueueControl[]>([]);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState('');
  const [change, setChange] = useState<QueueChange | null>(null);
  const canManage = principal?.role !== 'viewer';

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const page = await api<Page<QueueControl>>('/v1/queues');
      setItems(page.items || []);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [api, notify]);

  useEffect(() => {
    load();
  }, [load]);

  const confirmChange = async () => {
    if (!change) return;
    const parsed = change.maxConcurrency.trim() ? Number(change.maxConcurrency) : null;
    if (parsed !== null && (!Number.isInteger(parsed) || parsed < 1)) {
      notify('error', 'Concurrency must be a positive whole number or left blank.');
      return;
    }
    setPending(change.queue.queue);
    try {
      await api(`/v1/queues/${encodeURIComponent(change.queue.queue)}`, {
        method: 'PUT',
        body: JSON.stringify({paused: change.paused, max_concurrency: parsed}),
      });
      notify(
        'success',
        change.kind === 'availability'
          ? `Queue ${change.paused ? 'paused' : 'resumed'}.`
          : 'Queue concurrency updated.',
      );
      setChange(null);
      await load();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending('');
    }
  };

  if (loading) return <LoadingState label="Loading queues" />;
  if (!items.length)
    return <EmptyState title="No queues found" message="Queues appear after work is submitted." />;

  return (
    <>
      <div className="panel table-panel">
        <table>
          <thead>
            <tr>
              <th>Queue</th>
              <th>Status</th>
              <th>Running</th>
              <th>Concurrency cap</th>
              <th>
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((queue) => (
              <tr key={queue.queue}>
                <td className="mono">{queue.queue}</td>
                <td>
                  <StatusBadge status={queue.paused ? 'paused' : 'running'} />
                </td>
                <td>{queue.running}</td>
                <td>{queue.max_concurrency ?? 'Unlimited'}</td>
                <td>
                  {canManage ? (
                    <div className="row-actions">
                      <button
                        className={queue.paused ? undefined : 'danger'}
                        disabled={Boolean(pending)}
                        onClick={() =>
                          setChange({
                            queue,
                            paused: !queue.paused,
                            maxConcurrency: queue.max_concurrency?.toString() ?? '',
                            kind: 'availability',
                          })
                        }
                      >
                        {queue.paused ? 'Resume' : 'Pause'}
                      </button>
                      <button
                        disabled={Boolean(pending)}
                        onClick={() =>
                          setChange({
                            queue,
                            paused: queue.paused,
                            maxConcurrency: queue.max_concurrency?.toString() ?? '',
                            kind: 'concurrency',
                          })
                        }
                      >
                        Configure
                      </button>
                    </div>
                  ) : (
                    <span className="muted">Read only</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {change && (
        <Dialog
          title={
            change.kind === 'availability'
              ? `${change.paused ? 'Pause' : 'Resume'} queue?`
              : 'Change concurrency cap?'
          }
          onClose={() => !pending && setChange(null)}
          actions={
            <>
              <button onClick={() => setChange(null)} disabled={Boolean(pending)}>
                Keep current settings
              </button>
              <button
                className={change.paused ? 'danger' : 'primary'}
                onClick={confirmChange}
                disabled={Boolean(pending)}
              >
                {pending ? 'Updating…' : 'Confirm change'}
              </button>
            </>
          }
        >
          <p>
            Queue <span className="mono">{change.queue.queue}</span>
            {change.paused
              ? ' will stop dispatching new work until it is resumed.'
              : ' will be allowed to dispatch work.'}
          </p>
          {change.kind === 'concurrency' && (
            <label>
              Maximum concurrent jobs
              <input
                type="number"
                min="1"
                placeholder="Unlimited"
                value={change.maxConcurrency}
                onChange={(event) =>
                  setChange((current) =>
                    current ? {...current, maxConcurrency: event.target.value} : current,
                  )
                }
              />
            </label>
          )}
          <p className="muted">Acting as {principal?.role}.</p>
        </Dialog>
      )}
    </>
  );
}
