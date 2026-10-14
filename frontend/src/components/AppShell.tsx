import type {AppShellProps, View} from '../types';
import {ThemeControl} from './ThemeControl';
import {ObservabilityLinks} from './ObservabilityLinks';

const views: View[] = ['Jobs', 'Workflows', 'DAG Builder', 'Schedules', 'Workers', 'Queues'];

export function AppShell({
  view,
  onViewChange,
  connected,
  principal,
  onDisconnect,
  children,
}: AppShellProps) {
  const visibleViews = principal?.role === 'admin' ? [...views, 'Administration' as const] : views;
  return (
    <div className="shell">
      <aside>
        <div className="sidebar-heading">
          <div className="brand">
            ForgeQueue <span>Console</span>
          </div>
          <span
            className={`connection-dot ${connected ? 'connected' : ''}`}
            role="status"
            aria-label={connected ? 'API connected' : 'API disconnected'}
          />
        </div>
        <nav aria-label="Primary navigation">
          {visibleViews.map((item) => (
            <button
              key={item}
              aria-current={view === item ? 'page' : undefined}
              className={view === item ? 'active' : ''}
              onClick={() => onViewChange(item)}
            >
              {item}
            </button>
          ))}
        </nav>
        {connected && (
          <div className="sidebar-footer">
            <span className="session-principal">
              {principal?.name}
              <small>{principal?.role}</small>
            </span>
            <button onClick={onDisconnect}>Disconnect</button>
          </div>
        )}
      </aside>
      <main id="main-content">
        <header>
          <div>
            <p className="eyebrow">Operations console</p>
            <h1>{view}</h1>
            <p>Distributed execution at a glance</p>
          </div>
          <span className={`connection ${connected ? 'connected' : ''}`}>
            {connected ? 'API connected' : 'Not connected'}
          </span>
        </header>
        <div className="toolbar">
          <ThemeControl />
          <ObservabilityLinks resource={view.toLowerCase()} />
        </div>
        {children}
      </main>
    </div>
  );
}
