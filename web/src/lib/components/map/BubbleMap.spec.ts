import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MapPlace } from '#lib/worldMap.js';
import BubbleMap from './BubbleMap.svelte';

const { page } = vi.hoisted(() => ({ page: { data: {} as { locale?: string }, url: new URL('http://localhost/jobs') } }));
vi.mock('$app/state', () => ({ page }));

const PLACES: MapPlace[] = [
  { key: 'br', label: 'Brazil', count: 12_345, lat: -10, lng: -52 },
  { key: 'mx', label: 'Mexico', count: 1, lat: 23, lng: -102 },
];

beforeEach(() => {
  page.data = {};
});

describe('BubbleMap — interactive', () => {
  it('draws every place as a named button carrying its count', () => {
    render(BubbleMap, { places: PLACES, label: 'Jobs map', onpick: vi.fn() });
    expect(screen.getByRole('button', { name: 'Brazil: 12,345 jobs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Mexico: 1 job' })).toBeTruthy();
  });

  it('picks a place on click, Enter and Space', async () => {
    const onpick = vi.fn();
    render(BubbleMap, { places: PLACES, label: 'Jobs map', onpick });
    const brazil = screen.getByRole('button', { name: /Brazil/ });
    await fireEvent.click(brazil);
    await fireEvent.keyDown(brazil, { key: 'Enter' });
    await fireEvent.keyDown(brazil, { key: ' ' });
    expect(onpick).toHaveBeenCalledTimes(3);
    expect(onpick).toHaveBeenNthCalledWith(3, PLACES[0]);
  });

  it('does not pick on other keys', async () => {
    const onpick = vi.fn();
    render(BubbleMap, { places: PLACES, label: 'Jobs map', onpick });
    await fireEvent.keyDown(screen.getByRole('button', { name: /Brazil/ }), { key: 'a' });
    expect(onpick).not.toHaveBeenCalled();
  });

  it('names the place and its count on hover', async () => {
    render(BubbleMap, { places: PLACES, label: 'Jobs map', onpick: vi.fn() });
    const brazil = screen.getByRole('button', { name: /Brazil/ });
    await fireEvent.pointerEnter(brazil);
    const tip = screen.getByRole('tooltip');
    expect(tip.textContent).toContain('Brazil');
    expect(tip.textContent).toContain('12,345 jobs');
    await fireEvent.pointerLeave(brazil);
    expect(screen.queryByRole('tooltip')).toBeNull();
  });

  it('names the place on keyboard focus too', async () => {
    render(BubbleMap, { places: PLACES, label: 'Jobs map', onpick: vi.fn() });
    const mexico = screen.getByRole('button', { name: /Mexico/ });
    await fireEvent.focus(mexico);
    expect(screen.getByRole('tooltip').textContent).toContain('Mexico');
    await fireEvent.blur(mexico);
    expect(screen.queryByRole('tooltip')).toBeNull();
  });
});

describe('BubbleMap — read-only', () => {
  it('offers nothing to click or focus', () => {
    const { container } = render(BubbleMap, { places: PLACES, label: 'Jobs map' });
    expect(screen.queryAllByRole('button')).toEqual([]);
    expect(container.querySelectorAll('circle')).toHaveLength(2);
    expect(container.querySelector('[tabindex]')).toBeNull();
    expect(screen.getByRole('img', { name: 'Jobs map' })).toBeTruthy();
  });

  it('names each place in its native tooltip', () => {
    const { container } = render(BubbleMap, { places: PLACES, label: 'Jobs map' });
    const titles = [...container.querySelectorAll('circle title')].map((t) => t.textContent);
    expect(titles).toEqual(['Brazil: 12,345 jobs', 'Mexico: 1 job']);
  });
});

describe('BubbleMap — Russian', () => {
  it('formats counts and plurals for the locale', () => {
    page.data = { locale: 'ru' };
    render(BubbleMap, { places: PLACES, label: 'Карта', onpick: vi.fn() });
    const brazil = screen.getByRole('button', { name: /Brazil/ });
    expect(brazil.getAttribute('aria-label')).toBe(`Brazil: ${(12_345).toLocaleString('ru')} вакансий`);
    expect(screen.getByRole('button', { name: 'Mexico: 1 вакансия' })).toBeTruthy();
  });
});
