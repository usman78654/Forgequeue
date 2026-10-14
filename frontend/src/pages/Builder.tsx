import {useEffect, useMemo, useState} from 'react';
import {Background, Controls, ReactFlow, type Edge, type Node} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {
  errorMessage,
  type JobType,
  type PageProps,
  type StepCondition,
  type Workflow,
  type WorkflowStepInput,
} from '../types';

export function Builder({api, notify, principal}: PageProps) {
  const canRun = principal?.role !== 'viewer';
  const [name, setName] = useState('visual-pipeline');
  const [key, setKey] = useState('');
  const [type, setType] = useState<JobType>('echo');
  const [dependencies, setDependencies] = useState('');
  const [condition, setCondition] = useState<StepCondition>('success()');
  const [steps, setSteps] = useState<WorkflowStepInput[]>([]);
  const [pending, setPending] = useState(false);
  const [payload, setPayload] = useState('{"message":"hello"}');
  const [editing, setEditing] = useState('');
  const [importText, setImportText] = useState('');
  const [dirty, setDirty] = useState(false);
  useEffect(() => {
    if (!dirty) return;
    const leave = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    const navigate = (event: Event) => {
      if (
        !window.confirm(
          'Discard unsaved workflow changes? Export your workflow before leaving to keep a copy.',
        )
      )
        event.preventDefault();
    };
    window.addEventListener('beforeunload', leave);
    window.addEventListener('forgequeue:before-navigation', navigate);
    return () => {
      window.removeEventListener('beforeunload', leave);
      window.removeEventListener('forgequeue:before-navigation', navigate);
    };
  }, [dirty]);
  const add = () => {
    const normalizedKey = key.trim();
    if (!normalizedKey) return notify('error', 'Enter a step key.');
    if (steps.some((step) => step.key === normalizedKey && step.key !== editing))
      return notify('error', 'Step keys must be unique.');
    if (editing && normalizedKey !== editing)
      return notify('error', 'Keep the existing key when editing a step.');
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(payload) as Record<string, unknown>;
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error();
    } catch {
      return notify('error', 'Step payload must be a JSON object.');
    }
    const dependsOn = dependencies
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
    const missing = dependsOn.find((dependency) => !steps.some((step) => step.key === dependency));
    if (missing) return notify('error', `Dependency “${missing}” does not exist yet.`);
    const updated = [
      ...steps.filter((step) => step.key !== editing),
      {
        key: normalizedKey,
        type,
        payload: parsed,
        depends_on: dependsOn,
        condition,
      },
    ];
    if (!validGraph(updated)) return notify('error', 'Dependencies must form an acyclic graph.');
    setSteps(updated);
    setDirty(true);
    setEditing('');
    setKey('');
    setDependencies('');
  };
  const run = async () => {
    if (!canRun) return notify('error', 'Viewers cannot submit workflows.');
    if (!steps.length) return notify('error', 'Add at least one step before running the workflow.');
    setPending(true);
    try {
      const workflow = await api<Workflow>('/v1/workflows', {
        method: 'POST',
        body: JSON.stringify({name, steps}),
      });
      notify('success', `Workflow ${workflow.id.slice(0, 8)} started.`);
      setDirty(false);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  return (
    <div className="builder">
      <section className="palette" aria-labelledby="workflow-settings">
        <h2 id="workflow-settings">Workflow</h2>
        <label>
          Name
          <input
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              setDirty(true);
            }}
          />
        </label>
        <label>
          Step key
          <input value={key} onChange={(event) => setKey(event.target.value)} />
        </label>
        <label>
          Job type
          <select value={type} onChange={(event) => setType(event.target.value as JobType)}>
            <option>echo</option>
            <option>sleep</option>
            <option>command</option>
          </select>
        </label>
        <label>
          Depends on
          <input
            value={dependencies}
            onChange={(event) => setDependencies(event.target.value)}
            placeholder="fetch, validate"
          />
        </label>
        <label>
          Condition
          <select
            value={condition}
            onChange={(event) => setCondition(event.target.value as StepCondition)}
          >
            <option>success()</option>
            <option>failure()</option>
            <option>always()</option>
          </select>
        </label>
        <label>
          Payload JSON
          <textarea value={payload} onChange={(event) => setPayload(event.target.value)} />
        </label>
        <button onClick={add}>{editing ? 'Save step' : 'Add step'}</button>
        {steps.map((step) => (
          <div className="row-actions" key={step.key}>
            <button
              aria-label={`Edit step ${step.key}`}
              onClick={() => {
                setEditing(step.key);
                setKey(step.key);
                setType(step.type);
                setDependencies(step.depends_on.join(', '));
                setCondition(step.condition);
                setPayload(JSON.stringify(step.payload, null, 2));
              }}
            >
              Edit {step.key}
            </button>
            <button
              aria-label={`Delete step ${step.key}`}
              onClick={() => {
                if (steps.some((item) => item.depends_on.includes(step.key)))
                  return notify('error', 'Remove dependent references before deleting this step.');
                setSteps((old) => old.filter((item) => item.key !== step.key));
                setDirty(true);
              }}
            >
              Delete
            </button>
          </div>
        ))}
        <label>
          Import workflow JSON
          <textarea value={importText} onChange={(event) => setImportText(event.target.value)} />
        </label>
        <button
          onClick={() => {
            try {
              const imported = JSON.parse(importText) as {name: string; steps: WorkflowStepInput[]};
              if (
                typeof imported.name !== 'string' ||
                !imported.name.trim() ||
                !Array.isArray(imported.steps) ||
                !validGraph(imported.steps)
              )
                throw new Error();
              setName(imported.name);
              setSteps(imported.steps);
              setDirty(true);
            } catch {
              notify('error', 'Import requires a named workflow with valid acyclic steps.');
            }
          }}
        >
          Import workflow
        </button>
        <button
          onClick={() => {
            const url = URL.createObjectURL(
              new Blob([JSON.stringify({name, steps}, null, 2)], {type: 'application/json'}),
            );
            const link = document.createElement('a');
            link.href = url;
            link.download = 'workflow.json';
            link.click();
            URL.revokeObjectURL(url);
          }}
        >
          Export workflow
        </button>
        <button className="primary" onClick={run} disabled={pending || !canRun}>
          {pending ? 'Starting…' : 'Run workflow'}
        </button>
      </section>
      <Graph steps={steps} />
      <pre aria-label="Workflow JSON preview">{JSON.stringify({name, steps}, null, 2)}</pre>
    </div>
  );
}

function validGraph(steps: WorkflowStepInput[]): boolean {
  if (steps.length > 100) return false;
  const byKey = new Map(steps.map((step) => [step.key, step]));
  if (byKey.size !== steps.length) return false;
  const complete = new Set<string>();
  const active = new Set<string>();
  const visit = (key: string): boolean => {
    if (active.has(key)) return false;
    if (complete.has(key)) return true;
    const step = byKey.get(key);
    if (
      !step ||
      typeof step.key !== 'string' ||
      !step.key.trim() ||
      !['echo', 'sleep', 'command'].includes(step.type) ||
      !['success()', 'failure()', 'always()'].includes(step.condition) ||
      !Array.isArray(step.depends_on) ||
      !step.payload ||
      typeof step.payload !== 'object' ||
      Array.isArray(step.payload)
    )
      return false;
    active.add(key);
    if (!step.depends_on.every((dependency) => typeof dependency === 'string' && visit(dependency)))
      return false;
    active.delete(key);
    complete.add(key);
    return true;
  };
  return steps.every((step) => visit(step.key));
}

function Graph({steps}: {steps: WorkflowStepInput[]}) {
  const nodes = useMemo<Node[]>(
    () =>
      steps.map((step, index) => ({
        id: step.key,
        position: {x: 40 + (index % 3) * 190, y: 40 + Math.floor(index / 3) * 130},
        data: {
          label: (
            <span>
              <b>{step.key}</b>
              <br />
              {step.type} · {step.condition}
            </span>
          ),
        },
      })),
    [steps],
  );
  const edges = useMemo<Edge[]>(
    () =>
      steps.flatMap((step) =>
        step.depends_on.map((dependency) => ({
          id: `${dependency}-${step.key}`,
          source: dependency,
          target: step.key,
          animated: true,
        })),
      ),
    [steps],
  );
  return (
    <div className="canvas" aria-label="Workflow dependency graph">
      <ReactFlow nodes={nodes} edges={edges} fitView>
        <Background />
        <Controls />
      </ReactFlow>
    </div>
  );
}
