import { readFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';
import { afterEach, describe, expect, it } from 'vitest';

const page = readFileSync(new URL('../../internal/api/mcpapp/widget/jobs.html', import.meta.url), 'utf8');
const instances = [];
const job = {
  slug: 'senior-go-engineer-acme-abc123',
  title: 'Senior Go Engineer',
  company: 'Acme',
  url: 'https://freehire.me/jobs/senior-go-engineer-acme-abc123',
  official_job_url: 'https://careers.acme.example/go',
  source: 'Acme careers',
};

function mount({ capabilities = { serverTools: {} }, posting = job } = {}) {
  const sent = [];
  const timers = [];
  const dom = new JSDOM(page, {
    url: 'https://mcp-host.example/widget',
    runScripts: 'dangerously',
    beforeParse(window) {
      window.postMessage = (message) => sent.push(message);
      window.ResizeObserver = class {
        observe() {}
        disconnect() {}
      };
      window.setTimeout = (callback, delay) => {
        timers.push({ callback, delay });
        return timers.length;
      };
      window.clearTimeout = () => {};
    },
  });
  instances.push(dom);
  const window = dom.window;
  const receive = (data, source = window) => window.dispatchEvent(new window.MessageEvent('message', { data, source }));
  const answer = (message, result) => receive({ jsonrpc: '2.0', id: message.id, result });
  const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
  const init = sent.find((message) => message.method === 'ui/initialize');
  answer(init, { hostCapabilities: capabilities, hostContext: { theme: 'light' } });
  receive({
    jsonrpc: '2.0',
    method: 'ui/notifications/tool-result',
    params: { structuredContent: { jobs: [posting], total: 1 } },
  });
  return {
    window,
    receive,
    answer,
    tick,
    sent,
    timers,
    button: (text) =>
      Array.from(window.document.querySelectorAll('button')).find((element) => element.textContent === text),
    calls: () => sent.filter((message) => message.method === 'tools/call'),
    status: () => window.document.querySelector('[role="status"]')?.textContent ?? '',
  };
}

afterEach(() => {
  for (const dom of instances.splice(0)) dom.window.close();
});

describe('MCP job-card actions over the host bridge', () => {
  it('saves the exact posting only after a click and waits for a confirmed result', async () => {
    const ui = mount();
    await ui.tick();
    expect(ui.calls()).toHaveLength(0);
    const save = ui.button('Save job');
    expect(save).toBeDefined();
    save.click();
    save.click();
    expect(ui.calls()).toHaveLength(1);
    expect(ui.calls()[0].params).toEqual({ name: 'save_job', arguments: { slug: job.slug } });
    expect(ui.button('Saved')).toBeUndefined();
    expect(ui.status()).toContain('Saving');
    ui.answer(ui.calls()[0], {
      content: [{ type: 'text', text: JSON.stringify({ job_id: 42, saved_at: '2026-11-05T03:00:00Z' }) }],
    });
    await ui.tick();
    expect(ui.button('Saved')?.disabled).toBe(true);
    expect(ui.status()).toBe('Saved to your freehire board.');
  });

  it('changes the chosen stage only on explicit update and never overwrites notes', async () => {
    const ui = mount();
    await ui.tick();
    const select = ui.window.document.querySelector('select');
    expect(select).not.toBeNull();
    expect(ui.button('Update stage')?.disabled).toBe(true);
    select.value = 'interview';
    select.dispatchEvent(new ui.window.Event('change'));
    expect(ui.calls()).toHaveLength(0);
    ui.button('Update stage').click();
    expect(ui.calls()[0].params).toEqual({ name: 'track_job', arguments: { slug: job.slug, stage: 'interview' } });
    expect(ui.button('Save job').disabled).toBe(true);
    ui.answer(ui.calls()[0], {
      structuredContent: { job_id: 42, stage: 'interview', notes: 'Existing private note' },
      content: [],
    });
    await ui.tick();
    expect(ui.status()).toBe('Stage updated to Interview.');
  });

  it('shows a failed tool response as a failure and allows a deliberate retry', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    ui.answer(ui.calls()[0], { isError: true, content: [{ type: 'text', text: 'Job no longer available.' }] });
    await ui.tick();
    expect(ui.status()).toContain('Job no longer available.');
    expect(ui.button('Saved')).toBeUndefined();
    expect(ui.button('Save job').disabled).toBe(false);
    ui.button('Save job').click();
    expect(ui.calls()).toHaveLength(2);
  });

  it('does not turn an empty success envelope into a false saved confirmation', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    ui.answer(ui.calls()[0], { content: [] });
    await ui.tick();
    expect(ui.status()).toContain('could not confirm');
    expect(ui.button('Saved')).toBeUndefined();
    expect(ui.button('Save job').disabled).toBe(true);
  });

  it('keeps an unconfirmed timed-out write disabled until the user checks their board', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    expect(ui.timers.some((timer) => timer.delay === 30000)).toBe(true);
    ui.timers.find((timer) => timer.delay === 30000).callback();
    await ui.tick();
    expect(ui.status()).toContain('could not confirm');
    expect(ui.status()).toContain('board');
    expect(ui.button('Save job').disabled).toBe(true);
    ui.answer(ui.calls()[0], { structuredContent: { job_id: 42, saved_at: '2026-11-05T03:00:00Z' } });
    await ui.tick();
    expect(ui.button('Saved')).toBeUndefined();
  });

  it('unlocks an unknown write only after no-change confirmation and ignores the old response', async () => {
    const ui = mount();
    await ui.tick();
    expect(ui.window.document.querySelector('.action-recovery')?.hidden).toBe(true);
    ui.button('Save job').click();
    const first = ui.calls()[0];
    ui.timers.find((timer) => timer.delay === 30000).callback();
    await ui.tick();
    ui.receive({
      jsonrpc: '2.0',
      method: 'ui/notifications/tool-result',
      params: { structuredContent: { jobs: [job], total: 1 } },
    });
    await ui.tick();
    const reset = ui.button('I checked, allow retry');
    expect(reset).toBeDefined();
    expect(reset.disabled).toBe(true);
    expect(ui.window.document.querySelector('.action-recovery').hidden).toBe(false);
    reset.click();
    expect(ui.button('Save job').disabled).toBe(true);
    const confirmation = ui.window.document.querySelector('input[type="checkbox"]');
    expect(confirmation.closest('label').textContent).toContain('this change was not applied');
    confirmation.checked = true;
    confirmation.dispatchEvent(new ui.window.Event('change'));
    expect(reset.disabled).toBe(false);
    reset.click();
    expect(ui.calls()).toHaveLength(1);
    expect(ui.button('Save job').disabled).toBe(false);
    expect(ui.window.document.querySelector('.action-recovery').hidden).toBe(true);
    ui.button('Save job').click();
    expect(ui.calls()).toHaveLength(2);
    const second = ui.calls()[1];
    expect(second.id).not.toBe(first.id);
    ui.answer(first, { content: [], structuredContent: { job_id: 42, saved_at: '2026-11-05T03:00:00Z' } });
    await ui.tick();
    expect(ui.button('Saved')).toBeUndefined();
    expect(ui.status()).toContain('Saving');
    ui.answer(second, { content: [], structuredContent: { job_id: 42, saved_at: '2026-11-05T03:01:00Z' } });
    await ui.tick();
    expect(ui.button('Saved')?.disabled).toBe(true);
  });

  it('stays readable when a host cannot proxy account actions', async () => {
    const ui = mount({ capabilities: {} });
    await ui.tick();
    expect(ui.window.document.body.textContent).toContain(job.title);
    expect(ui.button('Save job')).toBeUndefined();
    expect(ui.window.document.querySelector('a[href="' + job.url + '"]')).not.toBeNull();
    expect(ui.calls()).toHaveLength(0);
  });

  it('renders untrusted posting data as text and never uses it as an action or URL', async () => {
    const ui = mount({
      posting: {
        ...job,
        slug: '',
        title: '<img src=x onerror=alert(1)>',
        url: 'javascript:alert(1)',
        official_job_url: 'http://unsafe.example/',
      },
    });
    await ui.tick();
    expect(ui.window.document.body.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(ui.window.document.querySelector('a')).toBeNull();
    expect(ui.button('Save job')).toBeUndefined();
    expect(ui.calls()).toHaveLength(0);
  });

  it('refuses a mismatched stage confirmation', async () => {
    const ui = mount();
    await ui.tick();
    const select = ui.window.document.querySelector('select');
    select.value = 'interview';
    select.dispatchEvent(new ui.window.Event('change'));
    ui.button('Update stage').click();
    ui.answer(ui.calls()[0], { structuredContent: { job_id: 42, stage: 'preparing' }, content: [] });
    await ui.tick();
    expect(ui.status()).toContain('could not confirm');
    expect(ui.button('Update stage').disabled).toBe(true);
  });

  it('handles malformed content without losing pending-state control', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    ui.answer(ui.calls()[0], { content: { text: 'malformed' } });
    await ui.tick();
    expect(ui.status()).toContain('could not confirm');
    expect(ui.button('Save job').disabled).toBe(true);
  });

  it('keeps a pending write locked across another search render', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    ui.receive({
      jsonrpc: '2.0',
      method: 'ui/notifications/tool-result',
      params: { structuredContent: { jobs: [job], total: 1 } },
    });
    await ui.tick();
    expect(ui.button('Saving...').disabled).toBe(true);
    ui.button('Saving...').click();
    expect(ui.calls()).toHaveLength(1);
    ui.answer(ui.calls()[0], { structuredContent: { job_id: 42, saved_at: '2026-11-05T03:00:00Z' }, content: [] });
    await ui.tick();
    expect(ui.button('Saved')?.disabled).toBe(true);
  });

  it('uses the backend application-stage vocabulary', async () => {
    const source = readFileSync(new URL('../../internal/application/userjob/stages.go', import.meta.url), 'utf8');
    const body = source.match(/var Stages = \[\]string\{([^}]+)\}/s)?.[1];
    expect(body).toBeDefined();
    const expected = Array.from(body.matchAll(/"([^"]+)"/g), (match) => match[1]);
    const ui = mount();
    await ui.tick();
    expect(Array.from(ui.window.document.querySelector('select').options, (option) => option.value)).toEqual([
      '',
      ...expected,
    ]);
  });

  it('ignores responses from another frame', async () => {
    const ui = mount();
    await ui.tick();
    ui.button('Save job').click();
    ui.receive(
      {
        jsonrpc: '2.0',
        id: ui.calls()[0].id,
        result: { structuredContent: { job_id: 42, saved_at: '2026-11-05T03:00:00Z' } },
      },
      null,
    );
    await ui.tick();
    expect(ui.button('Saved')).toBeUndefined();
    expect(ui.status()).toContain('Saving');
  });
});
