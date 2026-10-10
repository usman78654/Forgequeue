import {useState, type FormEvent} from 'react';
import {errorMessage} from '../types';

interface ConnectionPanelProps {
  onConnect: (apiKey: string) => Promise<void>;
}

export function ConnectionPanel({onConnect}: ConnectionPanelProps) {
  const [key, setKey] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!key.trim()) return setError('Enter an API key.');
    setPending(true);
    setError('');
    try {
      await onConnect(key.trim());
      setKey('');
    } catch (requestError: unknown) {
      setError(errorMessage(requestError));
    } finally {
      setPending(false);
    }
  };

  return (
    <section className="connect-panel" aria-labelledby="connect-title">
      <div>
        <span className="kicker">Secure session</span>
        <h2 id="connect-title">Connect to ForgeQueue</h2>
        <p>
          Your API key is exchanged once for an expiring HttpOnly session and is never stored by the
          browser application.
        </p>
      </div>
      <form onSubmit={submit} aria-busy={pending}>
        <label htmlFor="api-key">API key</label>
        <input
          id="api-key"
          type="password"
          autoComplete="off"
          value={key}
          onChange={(event) => setKey(event.target.value)}
          aria-describedby={error ? 'connection-error' : 'connection-help'}
        />
        <small id="connection-help">Use a key with the minimum role required for your task.</small>
        {error && (
          <p id="connection-error" className="field-error" role="alert">
            {error}
          </p>
        )}
        <button className="primary" disabled={pending}>
          {pending ? 'Connecting…' : 'Connect'}
        </button>
      </form>
    </section>
  );
}
