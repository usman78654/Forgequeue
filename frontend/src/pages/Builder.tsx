import {useMemo, useState} from 'react';
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

export function Builder({api, notify}: PageProps) {
  const [name, setName] = useState('visual-pipeline');
  const [key, setKey] = useState('');
  const [type, setType] = useState<JobType>('echo');
  const [dependencies, setDependencies] = useState('');
  const [condition, setCondition] = useState<StepCondition>('success()');
  const [steps, setSteps] = useState<WorkflowStepInput[]>([]);
  const [pending, setPending] = useState(false);
  const add = () => {
    const normalizedKey = key.trim();
    if (!normalizedKey) return notify('error', 'Enter a step key.');
    if (steps.some((step) => step.key === normalizedKey))
      return notify('error', 'Step keys must be unique.');
    const dependsOn = dependencies
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
    const missing = dependsOn.find((dependency) => !steps.some((step) => step.key === dependency));
    if (missing) return notify('error', `Dependency “${missing}” does not exist yet.`);
    setSteps((current) => [
      ...current,
      {
        key: normalizedKey,
        type,
        payload: {message: normalizedKey},
        depends_on: dependsOn,
        condition,
      },
    ]);
    setKey('');
    setDependencies('');
  };
  const run = async () => {
    if (!steps.length) return notify('error', 'Add at least one step before running the workflow.');
    setPending(true);
    try {
      const workflow = await api<Workflow>('/v1/workflows', {
        method: 'POST',
        body: JSON.stringify({name, steps}),
      });
      notify('success', `Workflow ${workflow.id.slice(0, 8)} started.`);
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
          <input value={name} onChange={(event) => setName(event.target.value)} />
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
        <button onClick={add}>Add step</button>
        <button className="primary" onClick={run} disabled={pending}>
          {pending ? 'Starting…' : 'Run workflow'}
        </button>
      </section>
      <Graph steps={steps} />
      <pre aria-label="Workflow JSON preview">{JSON.stringify({name, steps}, null, 2)}</pre>
    </div>
  );
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
