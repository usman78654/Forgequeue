import {useCallback, useEffect, useMemo, useState} from 'react';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {
  errorMessage,
  type Job,
  type JobDetail,
  type JobEvent,
  type JobStatus,
  type JobType,
  type Notify,
  type Page,
  type PageProps,
  type RetryPolicy,
} from '../types';
import type {ApiRequest} from '../api-client';

const states: JobStatus[] = ['queued', 'running', 'succeeded', 'retrying', 'dead'];

export function Jobs({api, notify, principal}: PageProps) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [filter, setFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [pendingJob, setPendingJob] = useState('');
  const [showSubmit, setShowSubmit] = useState(false);
  const [detail, setDetail] = useState<JobDetail | null>(null);
  const [confirmJob, setConfirmJob] = useState<Job | null>(null);
  const [selectedDead, setSelectedDead] = useState<Set<string>>(new Set());
  const [confirmBulk, setConfirmBulk] = useState(false);
  const canMutate = principal?.role !== 'viewer';

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const page = await api<Page<Job>>(
        `/v1/jobs?limit=50${filter ? `&status=${encodeURIComponent(filter)}` : ''}`,
      );
      setJobs(page.items || []);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [api, filter, notify]);

  useEffect(() => {
    load();
  }, [load]);
  const counts = useMemo(
    () =>
      (['queued', 'running', 'succeeded', 'dead'] as JobStatus[]).map(
        (state) => [state, jobs.filter((job) => job.status === state).length] as const,
      ),
    [jobs],
  );

  const inspect = async (job: Job) => {
    setPendingJob(job.id);
    try {
      const [current, events] = await Promise.all([
        api<Job>(`/v1/jobs/${job.id}`),
        api<Page<JobEvent>>(`/v1/jobs/${job.id}/events`),
      ]);
      setDetail({...current, events: events.items});
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPendingJob('');
    }
  };

  const mutate = async (job: Job) => {
    const action = job.status === 'dead' ? 'requeue' : 'cancel';
    setPendingJob(job.id);
    try {
      await api(`/v1/jobs/${job.id}/${action}`, {
        method: 'POST',
        body: action === 'cancel' ? '{}' : undefined,
      });
      notify('success', action === 'requeue' ? 'Job requeued.' : 'Job cancellation requested.');
      await load();
      setConfirmJob(null);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPendingJob('');
    }
  };

  const bulkRequeue = async () => {
    const ids = [...selectedDead];
    if (!ids.length) return;
    setPendingJob('bulk');
    try {
      const results = await Promise.allSettled(
        ids.map((id) => api(`/v1/jobs/${id}/requeue`, {method: 'POST'})),
      );
      const failures = results.filter((result) => result.status === 'rejected');
      notify(
        failures.length ? 'error' : 'success',
        failures.length
          ? `${ids.length - failures.length} jobs requeued; ${failures.length} could not be requeued.`
          : `${ids.length} dead ${ids.length === 1 ? 'job' : 'jobs'} requeued.`,
      );
      setSelectedDead(new Set());
      setConfirmBulk(false);
      await load();
    } finally {
      setPendingJob('');
    }
  };

  const toggleDead = (id: string) =>
    setSelectedDead((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <>
      <div className="stats" aria-label="Job status summary">
        {counts.map(([state, count]) => (
          <div className="stat" key={state}>
            <span>{state}</span>
            <strong>{count}</strong>
          </div>
        ))}
      </div>
      <div className="toolbar">
        <label className="sr-only" htmlFor="job-status">
          Filter jobs by status
        </label>
        <select id="job-status" value={filter} onChange={(event) => setFilter(event.target.value)}>
          <option value="">All states</option>
          {states.map((state) => (
            <option key={state}>{state}</option>
          ))}
        </select>
        <button onClick={load} disabled={loading}>
          Refresh
        </button>
        {canMutate && selectedDead.size > 0 && (
          <button className="danger" onClick={() => setConfirmBulk(true)}>
            Retry selected ({selectedDead.size})
          </button>
        )}
        {canMutate && (
          <button className="primary" onClick={() => setShowSubmit(true)}>
            Submit job
          </button>
        )}
      </div>
      {loading ? (
        <LoadingState label="Loading jobs" />
      ) : jobs.length === 0 ? (
        <EmptyState
          title="No jobs found"
          message={
            filter
              ? 'Try another status or submit a new job.'
              : 'Submit a job to start processing work.'
          }
        />
      ) : (
        <div className="panel table-panel">
          <table>
            <thead>
              <tr>
                {canMutate && (
                  <th>
                    <span className="sr-only">Select dead jobs</span>
                  </th>
                )}
                <th>ID</th>
                <th>Type</th>
                <th>Queue</th>
                <th>Status</th>
                <th>Attempts</th>
                <th>Created</th>
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id}>
                  {canMutate && (
                    <td>
                      {job.status === 'dead' && (
                        <input
                          className="row-checkbox"
                          type="checkbox"
                          aria-label={`Select job ${job.id.slice(0, 8)}`}
                          checked={selectedDead.has(job.id)}
                          disabled={Boolean(pendingJob)}
                          onChange={() => toggleDead(job.id)}
                        />
                      )}
                    </td>
                  )}
                  <td>
                    <button
                      className="link-button"
                      disabled={pendingJob === job.id}
                      onClick={() => inspect(job)}
                    >
                      {job.id.slice(0, 8)}
                    </button>
                  </td>
                  <td>{job.type}</td>
                  <td>{job.queue}</td>
                  <td>
                    <StatusBadge status={job.status} />
                  </td>
                  <td>
                    {job.attempt}/{job.max_attempts}
                  </td>
                  <td>{new Date(job.created_at).toLocaleString()}</td>
                  <td>
                    {canMutate ? (
                      <button disabled={Boolean(pendingJob)} onClick={() => setConfirmJob(job)}>
                        {pendingJob === job.id
                          ? 'Working…'
                          : job.status === 'dead'
                            ? 'Retry'
                            : 'Cancel'}
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
      {detail && (
        <Dialog
          title={`Job ${detail.id.slice(0, 8)}`}
          onClose={() => setDetail(null)}
          actions={<button onClick={() => setDetail(null)}>Close</button>}
        >
          <StatusBadge status={detail.status} />
          <pre>{JSON.stringify(detail, null, 2)}</pre>
        </Dialog>
      )}
      {confirmJob && (
        <Dialog
          title={confirmJob.status === 'dead' ? 'Retry dead job?' : 'Cancel job?'}
          onClose={() => !pendingJob && setConfirmJob(null)}
          actions={
            <>
              <button onClick={() => setConfirmJob(null)} disabled={Boolean(pendingJob)}>
                Keep job
              </button>
              <button
                className="danger"
                onClick={() => mutate(confirmJob)}
                disabled={Boolean(pendingJob)}
              >
                {pendingJob
                  ? 'Working…'
                  : confirmJob.status === 'dead'
                    ? 'Confirm retry'
                    : 'Confirm cancellation'}
              </button>
            </>
          }
        >
          <p>
            {confirmJob.status === 'dead'
              ? 'This creates another delivery attempt for a job that exhausted its retries.'
              : 'Running work may continue briefly until the worker observes the cancellation.'}
          </p>
          <p className="muted">
            Job <span className="mono">{confirmJob.id.slice(0, 8)}</span>
            {principal ? ` · acting as ${principal.role}` : ''}
          </p>
        </Dialog>
      )}
      {confirmBulk && (
        <Dialog
          title="Retry selected dead jobs?"
          onClose={() => !pendingJob && setConfirmBulk(false)}
          actions={
            <>
              <button onClick={() => setConfirmBulk(false)} disabled={Boolean(pendingJob)}>
                Keep jobs dead
              </button>
              <button className="danger" onClick={bulkRequeue} disabled={Boolean(pendingJob)}>
                {pendingJob ? 'Requeueing…' : `Confirm retry of ${selectedDead.size}`}
              </button>
            </>
          }
        >
          <p>
            This creates another delivery attempt for {selectedDead.size} dead{' '}
            {selectedDead.size === 1 ? 'job' : 'jobs'}.
          </p>
          <p className="muted">Acting as {principal?.role || 'operator'}.</p>
        </Dialog>
      )}
      {showSubmit && (
        <SubmitJobDialog
          api={api}
          notify={notify}
          onClose={() => setShowSubmit(false)}
          onCreated={load}
        />
      )}
    </>
  );
}

interface SubmitJobDialogProps {
  api: ApiRequest;
  notify: Notify;
  onClose: () => void;
  onCreated: () => Promise<void>;
}

function SubmitJobDialog({api, notify, onClose, onCreated}: SubmitJobDialogProps) {
  const [type, setType] = useState<JobType>('echo');
  const [payload, setPayload] = useState('{"message":"hello from console"}');
  const [policy, setPolicy] = useState<RetryPolicy>('exponential');
  const [validation, setValidation] = useState('');
  const [pending, setPending] = useState(false);

  const submit = async () => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(payload);
    } catch {
      return setValidation('Payload must be valid JSON.');
    }
    setValidation('');
    setPending(true);
    try {
      await api('/v1/jobs', {
        method: 'POST',
        body: JSON.stringify({type, payload: parsed, retry_policy: policy}),
      });
      notify('success', 'Job submitted.');
      onClose();
      await onCreated();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog
      title="Submit job"
      onClose={onClose}
      actions={
        <>
          <button onClick={onClose} disabled={pending}>
            Cancel
          </button>
          <button className="primary" onClick={submit} disabled={pending}>
            {pending ? 'Submitting…' : 'Submit'}
          </button>
        </>
      }
    >
      <label>
        Type
        <select value={type} onChange={(event) => setType(event.target.value as JobType)}>
          <option>echo</option>
          <option>sleep</option>
          <option>command</option>
        </select>
      </label>
      <label>
        Payload
        <textarea
          value={payload}
          onChange={(event) => setPayload(event.target.value)}
          aria-invalid={Boolean(validation)}
          aria-describedby={validation ? 'payload-error' : undefined}
        />
      </label>
      {validation && (
        <span id="payload-error" className="field-error" role="alert">
          {validation}
        </span>
      )}
      <label>
        Retry policy
        <select value={policy} onChange={(event) => setPolicy(event.target.value as RetryPolicy)}>
          <option>exponential</option>
          <option>linear</option>
          <option>fixed</option>
        </select>
      </label>
    </Dialog>
  );
}
