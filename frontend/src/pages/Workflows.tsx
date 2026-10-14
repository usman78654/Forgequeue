import {useState} from 'react';
import {usePageQuery} from '../hooks/usePageQuery';
import {Pagination} from '../components/Pagination';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {errorMessage, type PageProps, type Workflow, type WorkflowSummary} from '../types';

export function Workflows({api, notify, principal}: PageProps) {
  const query = usePageQuery<WorkflowSummary>(api, '/v1/workflows?limit=50', notify);
  const {items, loading, load} = query;
  const [detail, setDetail] = useState<Workflow | null>(null);
  const [selected, setSelected] = useState<WorkflowSummary | null>(null);
  const [pending, setPending] = useState(false);
  const canMutate = principal?.role !== 'viewer';

  const inspect = async (id: string) => {
    setPending(true);
    try {
      setDetail(await api<Workflow>(`/v1/workflows/${id}`));
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };

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
            <button onClick={() => inspect(workflow.id)} disabled={pending}>
              Inspect steps
            </button>
            {canMutate && ['pending', 'running'].includes(workflow.status) && (
              <button className="danger card-action" onClick={() => setSelected(workflow)}>
                Cancel workflow
              </button>
            )}
          </article>
        ))}
      </div>
      <Pagination {...query} />
      {detail && (
        <Dialog title={`Workflow ${detail.name}`} onClose={() => setDetail(null)}>
          <div className="steps">
            {detail.steps.map((step) => (
              <span className="step" key={step.key}>
                {step.key}: {step.status}
              </span>
            ))}
          </div>
        </Dialog>
      )}
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
