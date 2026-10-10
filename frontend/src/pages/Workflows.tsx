import {useCallback, useEffect, useState} from 'react';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {
  errorMessage,
  type Page,
  type PageProps,
  type Workflow,
  type WorkflowSummary,
} from '../types';

export function Workflows({api, notify, principal}: PageProps) {
  const [items, setItems] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Workflow | null>(null);
  const [pending, setPending] = useState(false);
  const canMutate = principal?.role !== 'viewer';

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const page = await api<Page<WorkflowSummary>>('/v1/workflows?limit=50');
      const workflows = await Promise.all(
        (page.items || []).map((workflow) => api<Workflow>(`/v1/workflows/${workflow.id}`)),
      );
      setItems(workflows);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [api, notify]);

  useEffect(() => {
    load();
  }, [load]);

  const cancel = async () => {
    if (!selected) return;
    setPending(true);
    try {
      await api(`/v1/workflows/${selected.id}/cancel`, {method: 'POST', body: '{}'});
      notify('success', 'Workflow cancellation requested.');
      setSelected(null);
      await load();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };

  if (loading) return <LoadingState label="Loading workflows" />;
  if (!items.length)
    return (
      <EmptyState title="No workflows found" message="Build a DAG to coordinate multiple jobs." />
    );
  return (
    <>
      <div className="cards">
        {items.map((workflow) => (
          <article className="card" key={workflow.id}>
            <div className="card-heading">
              <h2>{workflow.name}</h2>
              <StatusBadge status={workflow.status} />
            </div>
            <p className="mono muted">{workflow.id.slice(0, 8)}</p>
            <div className="steps">
              {workflow.steps.map((step) => (
                <span className="step" key={step.key}>
                  {step.key}: {step.status}
                </span>
              ))}
            </div>
            {canMutate && ['pending', 'running'].includes(workflow.status) && (
              <button className="danger card-action" onClick={() => setSelected(workflow)}>
                Cancel workflow
              </button>
            )}
          </article>
        ))}
      </div>
      {selected && (
        <Dialog
          title="Cancel workflow?"
          onClose={() => !pending && setSelected(null)}
          actions={
            <>
              <button onClick={() => setSelected(null)} disabled={pending}>
                Keep workflow
              </button>
              <button className="danger" onClick={cancel} disabled={pending}>
                {pending ? 'Cancelling…' : 'Confirm cancellation'}
              </button>
            </>
          }
        >
          <p>
            Running jobs in <strong>{selected.name}</strong> will receive cancellation requests and
            blocked steps will not start.
          </p>
          <p className="muted">Acting as {principal?.role || 'operator'}.</p>
        </Dialog>
      )}
    </>
  );
}
