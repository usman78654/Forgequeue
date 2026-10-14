import {useCallback, useEffect, useState} from 'react';
import {Dialog} from '../components/Dialog';
import {EmptyState, LoadingState, StatusBadge} from '../components/Feedback';
import {errorMessage, type APIKeyRecord, type Page, type PageProps} from '../types';
import {AdminTools} from './AdminTools';

export function Administration({api, notify, principal}: PageProps) {
  const [items, setItems] = useState<APIKeyRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState('');
  const [selected, setSelected] = useState<APIKeyRecord | null>(null);

  const load = useCallback(
    async (showLoading = true) => {
      if (principal?.role !== 'admin') {
        setLoading(false);
        return;
      }
      if (showLoading) setLoading(true);
      try {
        const page = await api<Page<APIKeyRecord>>('/v1/admin/keys');
        setItems(page.items || []);
      } catch (error: unknown) {
        notify('error', errorMessage(error));
      } finally {
        setLoading(false);
      }
    },
    [api, notify, principal?.role],
  );

  useEffect(() => {
    load();
  }, [load]);

  const revoke = async () => {
    if (!selected) return;
    setPending(selected.key.id);
    try {
      await api(`/v1/admin/keys/${selected.key.id}`, {method: 'DELETE'});
      notify(
        'success',
        'API key revoked. Existing browser sessions remain independently revocable.',
      );
      setSelected(null);
      await load();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending('');
    }
  };

  if (principal?.role !== 'admin')
    return (
      <EmptyState title="Admin access required" message="This page is only available to admins." />
    );
  if (loading) return <LoadingState label="Loading API keys" />;

  return (
    <>
      <AdminTools
        api={api}
        notify={notify}
        principal={principal}
        onKeyCreated={() => load(false)}
      />
      <div className="panel table-panel">
        <table>
          <thead>
            <tr>
              <th>Principal</th>
              <th>Role</th>
              <th>Key</th>
              <th>Prefix</th>
              <th>Status</th>
              <th>Last used</th>
              <th>
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.key.id}>
                <td>{item.principal.name}</td>
                <td>{item.principal.role}</td>
                <td>{item.key.name}</td>
                <td className="mono">{item.key.prefix}…</td>
                <td>
                  <StatusBadge status={item.key.revoked_at ? 'revoked' : 'active'} />
                </td>
                <td>
                  {item.key.last_used_at
                    ? new Date(item.key.last_used_at).toLocaleString()
                    : 'Never'}
                </td>
                <td>
                  <button
                    className="danger"
                    disabled={Boolean(item.key.revoked_at) || Boolean(pending)}
                    onClick={() => setSelected(item)}
                  >
                    {item.key.revoked_at ? 'Revoked' : 'Revoke'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selected && (
        <Dialog
          title="Revoke API key?"
          onClose={() => !pending && setSelected(null)}
          actions={
            <>
              <button onClick={() => setSelected(null)} disabled={Boolean(pending)}>
                Keep key
              </button>
              <button className="danger" onClick={revoke} disabled={Boolean(pending)}>
                {pending ? 'Revoking…' : 'Confirm revocation'}
              </button>
            </>
          }
        >
          <p>
            Key <strong>{selected.key.name}</strong> ({selected.key.prefix}…) for{' '}
            <strong>{selected.principal.name}</strong> will stop authenticating new requests.
          </p>
          <p className="muted">This action is recorded in the audit log. Acting as admin.</p>
        </Dialog>
      )}
    </>
  );
}
