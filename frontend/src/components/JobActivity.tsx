import {useEffect, useState} from 'react';
import type {JobEvent} from '../types';

export function JobActivity({jobId, initialEvents}: {jobId: string; initialEvents: JobEvent[]}) {
  const [events, setEvents] = useState(initialEvents || []);
  const [connection, setConnection] = useState('Connecting');
  useEffect(() => {
    const after = Math.max(0, ...(initialEvents || []).map((event) => event.id));
    if (typeof EventSource === 'undefined') {
      setConnection('Live streaming unavailable');
      return;
    }
    const stream = new EventSource(
      `/v1/jobs/${encodeURIComponent(jobId)}/events/stream?after=${after}`,
    );
    stream.onopen = () => setConnection('Live');
    stream.onerror = () => setConnection(navigator.onLine ? 'Reconnecting' : 'Offline');
    const receive = (message: MessageEvent<string>) => {
      try {
        const event = JSON.parse(message.data) as JobEvent;
        if (!Number.isSafeInteger(event.id) || event.job_id !== jobId) return;
        setEvents((current) =>
          current.some((item) => item.id === event.id)
            ? current
            : [...current, event].sort((a, b) => a.id - b.id).slice(-1000),
        );
      } catch {
        setConnection('Invalid event received');
      }
    };
    for (const kind of ['state', 'progress', 'log'])
      stream.addEventListener(kind, receive as EventListener);
    return () => stream.close();
  }, [jobId, initialEvents]);
  const progress = [...events]
    .reverse()
    .find((event) => typeof event.progress === 'number')?.progress;
  return (
    <section aria-label="Job activity">
      <h3>Live activity</h3>
      <p role="status">{connection} · Retaining the newest 1,000 events</p>
      {typeof progress === 'number' && (
        <label>
          Progress
          <progress max={1} value={Math.max(0, Math.min(1, progress))} />
        </label>
      )}
      {events.length ? (
        <ol>
          {events.map((event) => (
            <li key={event.id}>
              <time>{new Date(event.created_at).toLocaleTimeString()}</time>{' '}
              <span>
                {event.level} · {event.type}
              </span>{' '}
              {event.message}
              {event.data != null && <pre>{JSON.stringify(event.data, null, 2)}</pre>}
            </li>
          ))}
        </ol>
      ) : (
        <p className="muted">No events yet.</p>
      )}
    </section>
  );
}
