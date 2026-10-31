import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { IngestStatus } from '#lib/types.js';
import StatusBoard from './StatusBoard.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/status'), state: {} },
}));

function status(over: Partial<IngestStatus> = {}): IngestStatus {
  return {
    overall: 'down',
    generated_at: '2026-10-31T12:00:00Z',
    site: {
      status: 'operational',
      database: 'up',
      error_rate: 0.012,
      pool_pressure: 0.4,
      window_minutes: 5,
      history: [
        { day: '2026-10-31', status: 'operational' },
        { day: '2026-10-30', status: 'operational' },
        { day: '2026-10-29', status: 'down' },
      ],
    },
    providers: [
      {
        provider: 'greenhouse',
        kind: 'ats',
        status: 'operational',
        total_boards: 10,
        healthy_boards: 8,
        cooled_boards: 1,
        last_run: '2026-10-31T11:00:00Z',
        last_success: '2026-10-31T11:00:00Z',
        ingested_total: 100,
      },
      {
        provider: 'lever',
        kind: 'ats',
        status: 'down',
        total_boards: 4,
        healthy_boards: 0,
        cooled_boards: 0,
        last_run: '2026-10-30T11:00:00Z',
        last_success: null,
        ingested_total: 0,
      },
    ],
    ...over,
  };
}

describe('StatusBoard', () => {
  it('says when the read failed', () => {
    render(StatusBoard, { status: null });
    expect(screen.getByText('Status is unavailable right now. Try again in a moment.')).toBeTruthy();
  });

  it('counts uptime only over days that have a sample', () => {
    render(StatusBoard, { status: status() });

    expect(screen.getByText('API operational')).toBeTruthy();
    expect(screen.getByText('66.7%')).toBeTruthy();
    expect(screen.getByText('uptime')).toBeTruthy();
    expect(screen.getByLabelText('Status calendar showing 90-day uptime data')).toBeTruthy();
    expect(screen.getByText('2026-08-03')).toBeTruthy();
    expect(screen.getByText('2026-10-31')).toBeTruthy();
    expect(screen.getByText(/Database connections in use: 40%/)).toBeTruthy();
  });

  it('hides the uptime figure when no day was sampled', () => {
    render(StatusBoard, { status: status({ site: { ...status().site, history: [] } }) });
    expect(screen.queryByText('uptime')).toBeNull();
    expect(screen.getByLabelText('Status calendar showing 90-day uptime data')).toBeTruthy();
  });

  it('shows each provider as a monitor row', async () => {
    render(StatusBoard, { status: status() });
    await fireEvent.click(screen.getByRole('tab', { name: 'Ingest fleet' }));

    expect(screen.getByText('Major outage')).toBeTruthy();
    expect(screen.getByText('2 providers monitored')).toBeTruthy();
    expect(screen.getByText('Greenhouse')).toBeTruthy();
    expect(screen.getByText('80%')).toBeTruthy();
    expect(screen.getByText(/8 \/ 10 boards healthy/)).toBeTruthy();
    expect(screen.getByText(/1 in cooldown/)).toBeTruthy();
  });
});
