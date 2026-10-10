import {lazy, Suspense, useCallback, useEffect, useMemo, useState, type ComponentType} from 'react';
import {createApiClient, exchangeApiKey, refreshWebSession, revokeWebSession} from './api-client';
import type {ApiError, WebSession} from './api-client';
import {AppShell} from './components/AppShell';
import {ConnectionPanel} from './components/ConnectionPanel';
import {ErrorBoundary} from './components/ErrorBoundary';
import {Notice} from './components/Feedback';
import {useOnlineStatus} from './hooks/useOnlineStatus';
import {Jobs} from './pages/Jobs';
import {Administration} from './pages/Administration';
import {Queues} from './pages/Queues';
import {Schedules} from './pages/Schedules';
import {Workers} from './pages/Workers';
import {Workflows} from './pages/Workflows';
import type {NoticeKind, NoticeMessage, PageProps, View} from './types';

const Builder = lazy(() => import('./pages/Builder').then((module) => ({default: module.Builder})));

const pages: Record<View, ComponentType<PageProps>> = {
  Jobs,
  Workflows,
  'DAG Builder': Builder,
  Schedules,
  Workers,
  Queues,
  Administration,
};

export default function App() {
  const [session, setSession] = useState<WebSession | null>(null);
  const [booting, setBooting] = useState(true);
  const [view, setView] = useState<View>('Jobs');
  const [notice, setNotice] = useState<NoticeMessage | null>(null);
  const [degradedMessage, setDegradedMessage] = useState('');
  const online = useOnlineStatus();
  const notify = useCallback((kind: NoticeKind, message: string) => setNotice({kind, message}), []);
  const handleApiError = useCallback(
    (error: ApiError) => {
      if (error.status === 401) {
        setSession(null);
        notify('error', 'Your session expired. Connect again to continue.');
      } else if ([502, 503, 504].includes(error.status)) {
        setDegradedMessage(error.message);
      }
    },
    [notify],
  );
  const api = useMemo(
    () => createApiClient(session?.csrf_token || '', handleApiError),
    [handleApiError, session],
  );
  useEffect(() => {
    let current = true;
    refreshWebSession()
      .then((active) => current && setSession(active))
      .catch(() => current && setSession(null))
      .finally(() => current && setBooting(false));
    return () => {
      current = false;
    };
  }, []);
  const connect = async (candidate: string) => {
    const active = await exchangeApiKey(candidate);
    setSession(active);
    notify('success', `Connected as ${active.principal.name}.`);
  };
  const disconnect = async () => {
    if (session) await revokeWebSession(session.csrf_token).catch(() => undefined);
    setSession(null);
    setView('Jobs');
    notify('info', 'Web session ended.');
  };
  const Page = pages[view];
  return (
    <AppShell
      view={view}
      onViewChange={setView}
      connected={Boolean(session)}
      principal={session?.principal}
      onDisconnect={disconnect}
    >
      <Notice notice={notice} onDismiss={() => setNotice(null)} />
      {!online && (
        <div className="notice error" role="alert">
          <span>You are offline. In-flight changes may not have reached ForgeQueue.</span>
        </div>
      )}
      {degradedMessage && (
        <div className="notice warning" role="status">
          <span>ForgeQueue is degraded: {degradedMessage}</span>
          <button
            className="icon-button"
            onClick={() => setDegradedMessage('')}
            aria-label="Dismiss degraded service warning"
          >
            ×
          </button>
        </div>
      )}
      {booting ? (
        <div className="state-card" role="status">
          Restoring secure session…
        </div>
      ) : session ? (
        <ErrorBoundary resetKey={`${view}:${session.principal.id}`}>
          <Suspense
            fallback={
              <div className="state-card" role="status">
                Loading page…
              </div>
            }
          >
            <Page api={api} notify={notify} principal={session.principal} />
          </Suspense>
        </ErrorBoundary>
      ) : (
        <ConnectionPanel onConnect={connect} />
      )}
    </AppShell>
  );
}
