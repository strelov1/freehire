import { describe, expect, it } from 'vitest';
import {
  hostRatingLine,
  hostSlotLabel,
  MOCK_TOPICS,
  isHttpsLink,
  mockCounterpartLabel,
  mockRoleLabel,
  mockTopicLabel,
  mockDateParts,
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
    expect(mockDateParts(mock, 'Europe/Berlin')).toEqual({ weekday: 'Tue', day: '10', month: 'Nov', time: '19:00–20:00' });
    expect(mockDateParts(mock, 'Asia/Tokyo')).toMatchObject({ weekday: 'Wed', day: '11', time: '03:00–04:00' });
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

describe('hostSlotLabel', () => {
  it('reads the wall clock the server resolved, without re-reading it in the browser zone', () => {
    // Tokyo's Thursday 01:00 is still Wednesday in UTC; the label must stay on Thursday.
    expect(hostSlotLabel('2026-11-05T01:00:00+09:00')).toBe('Thu 5 Nov, 01:00');
    expect(hostSlotLabel('2026-11-04T19:00:00-05:00')).toBe('Wed 4 Nov, 19:00');
  });

  it('falls back to the raw value it cannot read', () => {
    expect(hostSlotLabel('soon')).toBe('soon');
  });
});

describe('hostRatingLine', () => {
  it('states the average and the completed mocks', () => {
    expect(hostRatingLine({ rating_count: 3, rating_avg: 4.6, completed: 5 })).toBe('4.6 · 5 mocks');
    expect(hostRatingLine({ rating_count: 1, rating_avg: 5, completed: 1 })).toBe('5.0 · 1 mock');
  });

  it('says nothing without a revealed rating', () => {
    expect(hostRatingLine({ rating_count: 0, rating_avg: 0, completed: 4 })).toBeNull();
  });
});
