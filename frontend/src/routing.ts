import type {View} from './types';

const routes: Record<View, string> = {
  Jobs: 'jobs',
  Workflows: 'workflows',
  'DAG Builder': 'builder',
  Schedules: 'schedules',
  Workers: 'workers',
  Queues: 'queues',
  Administration: 'administration',
};
export function currentView(): View {
  const route = window.location.hash.slice(2).split('/')[0];
  return (Object.keys(routes) as View[]).find((view) => routes[view] === route) || 'Jobs';
}
export function navigate(view: View) {
  window.location.hash = `/${routes[view]}`;
}

export function allowNavigation(): boolean {
  return window.dispatchEvent(new Event('forgequeue:before-navigation', {cancelable: true}));
}

export function restoreRoute(view: View) {
  window.history.replaceState(null, '', `#/${routes[view]}`);
}
