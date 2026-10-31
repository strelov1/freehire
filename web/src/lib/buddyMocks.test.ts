import { describe, expect, it } from 'vitest';
import {
  MOCK_TOPICS,
  isHttpsLink,
  mockCounterpartLabel,
  mockRoleLabel,
  mockTopicLabel,
  mockWhen,
  reviewSummaryLine,
} from './buddyMocks';

describe('buddy mocks', () => {
  it('labels the three topics in the server order', () => {
    expect(MOCK_TOPICS.map(mockTopicLabel)).toEqual(['Coding', 'System design', 'Behavioral']);
  });

  it('accepts only an https link with a host', () => {
    expect(isHttpsLink(' https://meet.google.com/abc-defg-hij ')).toBe(true);
    expect(isHttpsLink('http://meet.google.com/abc')).toBe(false);
    expect(isHttpsLink('https://')).toBe(false);
    expect(isHttpsLink('meet.google.com/abc')).toBe(false);
  });

  // A mock carries instants, so rendering one in the reader's zone is safe (unlike a slot
  // label, which the server already resolved).
  it('renders a mock in the given zone', () => {
    const mock = { starts_at: '2026-11-10T18:00:00Z', ends_at: '2026-11-10T19:00:00Z' };
    const berlin = mockWhen(mock, 'Europe/Berlin');
    expect(berlin).toContain('10 November');
    expect(berlin).toContain('19:00');
    expect(berlin).toContain('20:00');
    expect(mockWhen(mock, 'Asia/Tokyo')).toContain('11 November');
  });

  it('never states an average of nothing', () => {
    expect(reviewSummaryLine({ completed: 0, rating_count: 0, rating_avg: 0 })).toBe('0 mock interviews · no ratings yet');
    expect(reviewSummaryLine({ completed: 1, rating_count: 1, rating_avg: 4 })).toBe('1 mock interview · 4.0 ★ from 1 rating');
    expect(reviewSummaryLine({ completed: 5, rating_count: 3, rating_avg: 4.333 })).toBe('5 mock interviews · 4.3 ★ from 3 ratings');
  });

  it('names a departed or blocked counterpart without a handle', () => {
    expect(mockCounterpartLabel({ counterpart: 'calm-otter-11' })).toBe('calm-otter-11');
    expect(mockCounterpartLabel({ counterpart: null })).toBe('A former buddy');
    expect(mockRoleLabel({ role: 'host' })).toBe('You host');
    expect(mockRoleLabel({ role: 'guest' })).toBe('You are the guest');
  });
});
