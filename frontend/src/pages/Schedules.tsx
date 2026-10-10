import {useCallback, useEffect, useState, type FormEvent} from 'react';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState} from '../components/Feedback';
import {errorMessage, type PageProps, type Schedule} from '../types';

export function Schedules({api, notify, principal}: PageProps) {
  const [items, setItems] = useState<Schedule[]>([]),
    [name, setName] = useState('hourly-demo'),
    [interval, setInterval] = useState('3600');
  const [loading, setLoading] = useState(true),
    [pending, setPending] = useState(false);
  const [selected, setSelected] = useState<Schedule | null>(null);
  const canMutate = principal?.role !== 'viewer';
  const load = useCallback(async () => {
    setLoading(true);
    try {
      setItems(await api<Schedule[]>('/v1/schedules'));
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [api, notify]);
  useEffect(() => {
    load();
  }, [load]);
  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!name.trim() || Number(interval) < 1)
      return notify('error', 'Enter a name and a positive interval.');
    setPending(true);
    try {
      await api('/v1/schedules', {
        method: 'POST',
        body: JSON.stringify({
          name: name.trim(),
          type: 'echo',
          payload: {scheduled: true},
          interval_seconds: Number(interval),
        }),
      });
      notify('success', 'Schedule created.');
      await load();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  const toggle = async () => {
    if (!selected) return;
    const action = selected.enabled ? 'disable' : 'enable';
    setPending(true);
    try {
      await api(`/v1/schedules/${selected.id}/${action}`, {method: 'POST'});
      notify('success', `Schedule ${selected.enabled ? 'disabled' : 'enabled'}.`);
      setSelected(null);
      await load();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  return (
    <>
      {canMutate && (
        <form className="toolbar schedule-form" onSubmit={create} aria-busy={pending}>
          <label>
            <span>Name</span>
            <input value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label>
            <span>Interval (seconds)</span>
            <input
              type="number"
              min="1"
              value={interval}
              onChange={(event) => setInterval(event.target.value)}
            />
          </label>
          <button className="primary" disabled={pending}>
            {pending ? 'Creating…' : 'Create schedule'}
          </button>
        </form>
      )}
      {loading ? (
        <LoadingState label="Loading schedules" />
      ) : !items.length ? (
        <EmptyState
          title="No schedules found"
          message="Create a recurring schedule with the form above."
        />
      ) : (
        <div className="panel table-panel">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Interval</th>
                <th>Enabled</th>
                <th>Next run</th>
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td>{item.name}</td>
                  <td>{item.type || item.job_type}</td>
                  <td>{item.interval_seconds}s</td>
                  <td>{item.enabled ? 'Yes' : 'No'}</td>
                  <td>{new Date(item.next_run_at).toLocaleString()}</td>
                  <td>
                    {canMutate ? (
                      <button
                        className={item.enabled ? 'danger' : undefined}
                        disabled={pending}
                        onClick={() => setSelected(item)}
                      >
                        {item.enabled ? 'Disable' : 'Enable'}
                      </button>
                    ) : (
                      <span className="muted">Read only</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {selected && (
        <Dialog
          title={`${selected.enabled ? 'Disable' : 'Enable'} schedule?`}
          onClose={() => !pending && setSelected(null)}
          actions={
            <>
              <button onClick={() => setSelected(null)} disabled={pending}>
                Keep current state
              </button>
              <button
                className={selected.enabled ? 'danger' : 'primary'}
                onClick={toggle}
                disabled={pending}
              >
                {pending ? 'Updating…' : `Confirm ${selected.enabled ? 'disable' : 'enable'}`}
              </button>
            </>
          }
        >
          <p>
            {selected.enabled
              ? 'No new jobs will be created by this schedule until it is enabled again.'
              : 'This schedule will resume creating jobs at its configured interval.'}
          </p>
          <p className="muted">
            Schedule <strong>{selected.name}</strong> · acting as {principal?.role || 'operator'}.
          </p>
        </Dialog>
      )}
    </>
  );
}
