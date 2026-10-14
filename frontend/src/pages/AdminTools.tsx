import {useEffect, useState, type FormEvent} from 'react';
import type {Principal} from '../api-client';
import {Dialog} from '../components/Dialog';
import {errorMessage, type Page, type PageProps} from '../types';

interface Audit {
  id: number;
  principal_id: string | null;
  action: string;
  path: string;
  status: number;
  created_at: string;
}

export function AdminTools({
  api,
  notify,
  principal,
  onKeyCreated,
}: PageProps & {onKeyCreated?: () => Promise<void>}) {
  const [name, setName] = useState('');
  const [role, setRole] = useState<Principal['role']>('operator');
  const [principalId, setPrincipalId] = useState('');
  const [keyName, setKeyName] = useState('');
  const [token, setToken] = useState('');
  const [pending, setPending] = useState(false);
  const [audit, setAudit] = useState<Audit[]>([]);
  const [search, setSearch] = useState('');
  const [quota, setQuota] = useState({
    submissions_per_minute: 60,
    max_concurrent_jobs: 100,
    max_payload_bytes: 1048576,
    max_schedules: 100,
  });
  const [confirmQuota, setConfirmQuota] = useState(false);
  useEffect(() => {
    if (principal?.role !== 'admin') return;
    const controller = new AbortController();
    api<Page<Audit>>('/v1/admin/audit', {signal: controller.signal})
      .then((page) => {
        if (!controller.signal.aborted) setAudit(page.items || []);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) notify('error', errorMessage(error));
      });
    return () => controller.abort();
  }, [api, notify, principal?.role]);
  const createPrincipal = async (event: FormEvent) => {
    event.preventDefault();
    setPending(true);
    try {
      const result = await api<Principal>('/v1/admin/principals', {
        method: 'POST',
        body: JSON.stringify({name: name.trim(), role}),
      });
      setPrincipalId(result.id);
      notify('success', `Principal ${result.name} created.`);
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  const createKey = async (event: FormEvent) => {
    event.preventDefault();
    setPending(true);
    try {
      const result = await api<{token: string}>(
        `/v1/admin/principals/${encodeURIComponent(principalId)}/keys`,
        {method: 'POST', body: JSON.stringify({name: keyName.trim()})},
      );
      setToken(result.token);
      await onKeyCreated?.();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  const saveQuota = async () => {
    if (Object.values(quota).some((value) => !Number.isInteger(value) || value < 1)) {
      notify('error', 'Quota values must be positive whole numbers.');
      return;
    }
    setPending(true);
    try {
      await api(`/v1/admin/quotas/${encodeURIComponent(principalId)}`, {
        method: 'PUT',
        body: JSON.stringify(quota),
      });
      setConfirmQuota(false);
      notify('success', 'Tenant quotas updated.');
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  return (
    <>
      <section className="card">
        <h2>Create principal</h2>
        <form onSubmit={createPrincipal}>
          <label>
            Principal name
            <input required value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label>
            Role
            <select
              value={role}
              onChange={(event) => setRole(event.target.value as Principal['role'])}
            >
              <option>viewer</option>
              <option>operator</option>
              <option>admin</option>
            </select>
          </label>
          <button disabled={pending || !name.trim()}>Create principal</button>
        </form>
      </section>
      <section className="card">
        <h2>Keys and quotas</h2>
        <label>
          Principal ID
          <input
            required
            value={principalId}
            onChange={(event) => setPrincipalId(event.target.value)}
          />
        </label>
        <form onSubmit={createKey}>
          <label>
            Key name
            <input required value={keyName} onChange={(event) => setKeyName(event.target.value)} />
          </label>
          <button disabled={pending || !principalId || !keyName.trim()}>Create API key</button>
        </form>
        {Object.entries(quota).map(([field, value]) => (
          <label key={field}>
            {field.replaceAll('_', ' ')}
            <input
              type="number"
              min="1"
              value={value}
              onChange={(event) =>
                setQuota((old) => ({...old, [field]: Number(event.target.value)}))
              }
            />
          </label>
        ))}
        <button disabled={pending || !principalId} onClick={() => setConfirmQuota(true)}>
          Update quotas
        </button>
      </section>
      {token && (
        <Dialog
          title="Save this API key"
          onClose={() => setToken('')}
          actions={<button onClick={() => setToken('')}>I have saved the key</button>}
        >
          <p>This token is shown once. Save it in your secret manager before closing.</p>
          <code>{token}</code>
        </Dialog>
      )}
      {confirmQuota && (
        <Dialog
          title="Update tenant quotas?"
          onClose={() => !pending && setConfirmQuota(false)}
          actions={
            <>
              <button disabled={pending} onClick={() => setConfirmQuota(false)}>
                Keep quotas
              </button>
              <button disabled={pending} onClick={saveQuota}>
                Confirm quota update
              </button>
            </>
          }
        >
          <p>
            These limits apply to principal {principalId}. Reducing limits may reject new
            submissions. Acting as admin.
          </p>
        </Dialog>
      )}
      <section className="card">
        <h2>Recent audit history</h2>
        <label>
          Search audit records
          <input value={search} onChange={(event) => setSearch(event.target.value)} />
        </label>
        <p className="muted">Newest 200 records. Search applies to loaded records.</p>
        <ol>
          {audit
            .filter((record) =>
              `${record.action} ${record.path} ${record.principal_id} ${record.status}`
                .toLowerCase()
                .includes(search.toLowerCase()),
            )
            .map((record) => (
              <li key={record.id}>
                {new Date(record.created_at).toLocaleString()} · {record.action} · {record.path} ·{' '}
                {record.status}
              </li>
            ))}
        </ol>
      </section>
    </>
  );
}
