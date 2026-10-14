import {useCallback, useEffect, useState} from 'react';
import type {ApiRequest} from '../api-client';
import {errorMessage, type Artifact, type Notify, type Page} from '../types';
import {Dialog} from './Dialog';

export function JobArtifacts({
  api,
  notify,
  jobId,
  canUpload,
}: {
  api: ApiRequest;
  notify: Notify;
  jobId: string;
  canUpload: boolean;
}) {
  const [items, setItems] = useState<Artifact[]>([]);
  const [pending, setPending] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [selected, setSelected] = useState<Artifact | null>(null);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const result = await api<Page<Artifact>>(`/v1/jobs/${jobId}/artifacts`, {signal});
        if (!signal?.aborted) setItems(result.items || []);
      } catch (error: unknown) {
        if (!signal?.aborted) notify('error', errorMessage(error));
      }
    },
    [api, notify, jobId],
  );
  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const upload = async () => {
    if (!file) return;
    if (file.size > 50 * 1024 * 1024) {
      notify('error', 'Choose a file smaller than 50 MiB.');
      return;
    }
    setPending(true);
    try {
      await api(`/v1/jobs/${jobId}/artifacts?name=${encodeURIComponent(file.name)}`, {
        method: 'POST',
        body: file,
        headers: {'Content-Type': file.type || 'application/octet-stream'},
      });
      setFile(null);
      await load();
      notify('success', 'Artifact uploaded.');
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  const download = async (artifact: Artifact) => {
    setPending(true);
    try {
      const result = await api<{url: string}>(
        `/v1/jobs/${jobId}/artifacts/${artifact.id}/download`,
      );
      const url = new URL(result.url);
      if (!['http:', 'https:'].includes(url.protocol)) throw new Error('Invalid download URL.');
      const link = document.createElement('a');
      link.href = url.href;
      link.rel = 'noopener noreferrer';
      link.target = '_blank';
      link.click();
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  const remove = async () => {
    if (!selected) return;
    setPending(true);
    try {
      await api(`/v1/jobs/${jobId}/artifacts/${selected.id}`, {method: 'DELETE'});
      setSelected(null);
      await load();
      notify('success', 'Artifact deleted.');
    } catch (error: unknown) {
      notify('error', errorMessage(error));
    } finally {
      setPending(false);
    }
  };
  return (
    <section aria-label="Job artifacts">
      <h3>Artifacts</h3>
      {canUpload && (
        <>
          <label>
            Upload artifact (up to 50 MiB)
            <input
              type="file"
              disabled={pending}
              onChange={(event) => setFile(event.target.files?.[0] || null)}
            />
          </label>
          <button onClick={upload} disabled={pending || !file}>
            {pending ? 'Transferring…' : 'Upload'}
          </button>
        </>
      )}
      {items.length ? (
        <ul>
          {items.map((artifact) => (
            <li key={artifact.id}>
              <button onClick={() => download(artifact)} disabled={pending}>
                {artifact.name}
              </button>{' '}
              {(artifact.size_bytes / 1024).toFixed(1)} KiB · expires{' '}
              {new Date(artifact.expires_at).toLocaleString()}
              {canUpload && (
                <button className="danger" disabled={pending} onClick={() => setSelected(artifact)}>
                  Delete {artifact.name}
                </button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p>No artifacts.</p>
      )}
      {selected && (
        <Dialog
          title="Delete artifact?"
          onClose={() => !pending && setSelected(null)}
          actions={
            <>
              <button disabled={pending} onClick={() => setSelected(null)}>
                Keep artifact
              </button>
              <button className="danger" disabled={pending} onClick={remove}>
                Confirm deletion
              </button>
            </>
          }
        >
          <p>Artifact {selected.name} will be permanently removed from object storage.</p>
        </Dialog>
      )}
    </section>
  );
}
