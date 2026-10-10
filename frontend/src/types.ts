import type {ApiRequest, Principal} from './api-client';
import type {ReactNode} from 'react';

export type View =
  'Jobs' | 'Workflows' | 'DAG Builder' | 'Schedules' | 'Workers' | 'Queues' | 'Administration';
export type NoticeKind = 'success' | 'error' | 'info';
export type Notify = (kind: NoticeKind, message: string) => void;

export interface NoticeMessage {
  kind: NoticeKind;
  message: string;
}

export interface PageProps {
  api: ApiRequest;
  notify: Notify;
  principal?: Principal;
}

export interface Page<T> {
  items: T[];
  next_cursor?: string;
}

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'retrying' | 'dead' | 'cancelled';
export type WorkflowStatus =
  'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'compensating' | 'compensated';
export type StepStatus = JobStatus | 'blocked' | 'skipped';
export type StatusValue =
  JobStatus | WorkflowStatus | StepStatus | 'healthy' | 'offline' | 'paused' | 'active' | 'revoked';
export type JobType = 'echo' | 'sleep' | 'command';
export type RetryPolicy = 'fixed' | 'linear' | 'exponential';
export type StepCondition = 'success()' | 'failure()' | 'always()';

export interface Job {
  id: string;
  type: string;
  queue: string;
  status: JobStatus;
  attempt: number;
  max_attempts: number;
  created_at: string;
}

export interface JobEvent {
  id: number;
  job_id: string;
  type: 'state' | 'progress' | 'log';
  level: 'debug' | 'info' | 'warn' | 'error';
  message?: string;
  progress?: number;
  data?: unknown;
  created_at: string;
}

export interface JobDetail extends Job {
  payload?: unknown;
  result?: unknown;
  error?: string;
  events: JobEvent[];
}

export interface WorkflowSummary {
  id: string;
  name: string;
  status: WorkflowStatus;
}

export interface WorkflowStep {
  key: string;
  status: StepStatus;
  job_id?: string;
}

export interface Workflow extends WorkflowSummary {
  steps: WorkflowStep[];
}

export interface WorkflowStepInput {
  key: string;
  type: JobType;
  payload: Record<string, unknown>;
  depends_on: string[];
  condition: StepCondition;
}

export interface Schedule {
  id: string;
  name: string;
  type?: string;
  job_type?: string;
  interval_seconds: number;
  enabled: boolean;
  next_run_at: string;
}

export interface Worker {
  id: string;
  queue: string;
  concurrency: number;
  healthy: boolean;
  last_heartbeat_at: string;
  queues?: string[];
  capabilities?: string[];
  state?: string;
}

export interface QueueControl {
  queue: string;
  paused: boolean;
  max_concurrency: number | null;
  running: number;
}

export interface APIKeyMetadata {
  id: string;
  name: string;
  prefix: string;
  expires_at: string | null;
  last_used_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface APIKeyRecord {
  key: APIKeyMetadata;
  principal: Principal;
}

export interface AppShellProps {
  view: View;
  onViewChange: (view: View) => void;
  connected: boolean;
  principal?: Principal;
  onDisconnect: () => void | Promise<void>;
  children: ReactNode;
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'An unexpected error occurred.';
}
