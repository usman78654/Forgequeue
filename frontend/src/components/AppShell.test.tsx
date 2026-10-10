import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {axe} from 'jest-axe';
import {describe, expect, it, vi} from 'vitest';
import {AppShell} from './AppShell';

describe('AppShell', () => {
  it('supports keyboard navigation and exposes the current page', async () => {
    const user = userEvent.setup();
    const onViewChange = vi.fn();
    render(
      <AppShell view="Jobs" onViewChange={onViewChange} connected={false} onDisconnect={vi.fn()}>
        <p>Content</p>
      </AppShell>,
    );

    const jobs = screen.getByRole('button', {name: 'Jobs'});
    const workflows = screen.getByRole('button', {name: 'Workflows'});
    expect(jobs).toHaveAttribute('aria-current', 'page');
    jobs.focus();
    await user.tab();
    expect(workflows).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(onViewChange).toHaveBeenCalledWith('Workflows');
  });

  it('has no automated accessibility violations', async () => {
    const {container} = render(
      <AppShell
        view="Jobs"
        onViewChange={vi.fn()}
        connected
        principal={{id: '1', name: 'Operator', role: 'operator'}}
        onDisconnect={vi.fn()}
      >
        <p>Content</p>
      </AppShell>,
    );
    expect((await axe(container)).violations).toEqual([]);
  });
});
