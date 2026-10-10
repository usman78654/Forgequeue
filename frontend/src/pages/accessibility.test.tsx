import {render, screen, waitFor} from '@testing-library/react';
import {axe} from 'jest-axe';
import {describe, expect, it, vi} from 'vitest';
import type {ReactElement} from 'react';
import type {ApiRequest} from '../api-client';
import type {Notify, PageProps} from '../types';
import {Builder} from './Builder';
import {Administration} from './Administration';
import {Jobs} from './Jobs';
import {Queues} from './Queues';
import {Schedules} from './Schedules';
import {Workers} from './Workers';
import {Workflows} from './Workflows';

const notify = vi.fn() as Notify;

function createApi(): ApiRequest {
  return (async (path: string) => {
    if (path === '/v1/schedules') return [];
    return {items: []};
  }) as ApiRequest;
}

const pages: Array<[string, (props: PageProps) => ReactElement]> = [
  ['Jobs', (props) => <Jobs {...props} />],
  ['Workflows', (props) => <Workflows {...props} />],
  ['DAG Builder', (props) => <Builder {...props} />],
  ['Schedules', (props) => <Schedules {...props} />],
  ['Workers', (props) => <Workers {...props} />],
  [
    'Queues',
    (props) => <Queues {...props} principal={{id: '1', name: 'Viewer', role: 'viewer'}} />,
  ],
  [
    'Administration',
    (props) => <Administration {...props} principal={{id: '1', name: 'Admin', role: 'admin'}} />,
  ],
];

describe.each(pages)('%s page accessibility', (_name, Page) => {
  it('has no automated accessibility violations', async () => {
    const {container} = render(<>{Page({api: createApi(), notify})}</>);
    await waitFor(() => expect(screen.queryByText(/^Loading/)).not.toBeInTheDocument());
    expect((await axe(container)).violations).toEqual([]);
  });
});
