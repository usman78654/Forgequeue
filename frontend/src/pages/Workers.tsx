import {useEffect, useState} from 'react';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {errorMessage, type Page, type PageProps, type Worker} from '../types';

export function Workers({api, notify}: PageProps) {
  const [items, setItems] = useState<Worker[]>([]);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let current = true;
    api<Page<Worker>>('/v1/workers')
      .then((page) => current && setItems(page.items || []))
      .catch((error: unknown) => current && notify('error', errorMessage(error)))
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [api, notify]);
  if (loading) return <LoadingState label="Loading workers" />;
  if (!items.length)
    return (
      <EmptyState
        title="No workers online"
        message="Start a worker to begin processing queued jobs."
      />
    );
  return (
    <div className="panel table-panel">
      <table>
        <thead>
          <tr>
            <th>Worker</th>
            <th>Queue</th>
            <th>Concurrency</th>
            <th>Health</th>
            <th>Heartbeat</th>
          </tr>
        </thead>
        <tbody>
          {items.map((worker) => (
            <tr key={worker.id}>
              <td className="mono">{worker.id}</td>
              <td>{worker.queue}</td>
              <td>{worker.concurrency}</td>
              <td>
                <StatusBadge status={worker.healthy ? 'healthy' : 'offline'} />
              </td>
              <td>{new Date(worker.last_heartbeat_at).toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
