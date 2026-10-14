import {useEffect, useState} from 'react';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {type PageProps, type Worker} from '../types';
import {usePageQuery} from '../hooks/usePageQuery';

export function Workers({api, notify}: PageProps) {
  const {items, loading, load} = usePageQuery<Worker>(api, '/v1/workers?', notify);
  const [filter, setFilter] = useState('');
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  if (loading) return <LoadingState label="Loading workers" />;
  if (!items.length)
    return (
      <EmptyState
        title="No workers online"
        message="Start a worker to begin processing queued jobs."
      />
    );
  return (
    <>
      <label>
        Filter workers, queues, state or capabilities
        <input value={filter} onChange={(event) => setFilter(event.target.value)} />
      </label>
      <button onClick={load} disabled={loading}>
        Refresh workers
      </button>
      <div className="panel table-panel">
        <table>
          <thead>
            <tr>
              <th>Worker</th>
              <th>Queue</th>
              <th>Capabilities</th>
              <th>State</th>
              <th>Concurrency</th>
              <th>Active jobs</th>
              <th>Health</th>
              <th>Heartbeat</th>
            </tr>
          </thead>
          <tbody>
            {items
              .filter((worker) =>
                [
                  worker.id,
                  worker.state || '',
                  ...(worker.queues || [worker.queue]),
                  ...(worker.capabilities || []),
                ]
                  .join(' ')
                  .toLowerCase()
                  .includes(filter.toLowerCase()),
              )
              .map((worker) => (
                <tr key={worker.id}>
                  <td className="mono">{worker.id}</td>
                  <td>{(worker.queues || [worker.queue]).join(', ')}</td>
                  <td>{worker.capabilities?.join(', ') || 'Unknown'}</td>
                  <td>{worker.state || 'Unknown'}</td>
                  <td>{worker.concurrency}</td>
                  <td>{worker.active_jobs ?? 'Unknown'}</td>
                  <td>
                    <StatusBadge
                      status={
                        worker.healthy && now - Date.parse(worker.last_heartbeat_at) < 30000
                          ? 'healthy'
                          : 'offline'
                      }
                    />
                  </td>
                  <td>
                    {new Date(worker.last_heartbeat_at).toLocaleString()} (
                    {Math.max(0, Math.floor((now - Date.parse(worker.last_heartbeat_at)) / 1000))}{' '}
                    seconds ago)
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
