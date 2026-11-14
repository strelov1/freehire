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
  const toolResult = (params) => receive({ jsonrpc: '2.0', method: 'ui/notifications/tool-result', params });
  const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
  const init = sent.find((message) => message.method === 'ui/initialize');
  answer(init, { hostCapabilities: capabilities, hostContext: { theme: 'light' } });
  toolResult({ structuredContent: { jobs: [posting], total: 1 } });
  return {
    window,
    receive,
    toolResult,
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

  it('keeps stage changes out of the widget', async () => {
    const ui = mount();
    await ui.tick();
    expect(ui.window.document.querySelector('select')).toBeNull();
    expect(ui.button('Update stage')).toBeUndefined();
    expect(page).not.toContain('track_job');
  });

  it.each([false, true])('renders profile skill coverage from the host (text response: %s)', async (textOnly) => {
    const ui = mount({ posting: { ...job, skills: ['go', 'sql', 'docker'] } });
    await ui.tick();
    expect(ui.calls()).toHaveLength(1);
    expect(ui.calls()[0].params).toEqual({ name: 'get_profile', arguments: {} });
    expect(ui.window.document.querySelector('.match')).toBeNull();
    const profile = { skills: ['GO', 'sql'] };
    ui.answer(
      ui.calls()[0],
      textOnly
        ? { content: [{ type: 'text', text: JSON.stringify(profile) }] }
        : { structuredContent: profile, content: [] },
    );
    await ui.tick();
    expect(ui.window.document.querySelector('.match-text').textContent).toBe('67% · 2/3 skills');
    expect(ui.window.document.querySelector('.sr-only').textContent).toBe('Profile match: 67%, 2 of 3 skills');
    expect(ui.window.document.querySelector('.match-fill').style.transform).toBe('translateX(-33%)');
    expect(ui.calls()).toHaveLength(1);
    expect(ui.button('Save job')).toBeDefined();
  });

  it('does not replace a newer tool error when an older profile read finishes', async () => {
    const ui = mount({ posting: { ...job, skills: ['go'] } });
    await ui.tick();
    const profileCall = ui.calls()[0];
    ui.toolResult({
      isError: true,
      content: [{ type: 'text', text: 'Search failed' }],
    });
    ui.answer(profileCall, { structuredContent: { skills: ['go'] }, content: [] });
    await ui.tick();
    expect(ui.window.document.querySelector('#root').textContent).toBe('Search failed');
    expect(ui.window.document.querySelector('.card')).toBeNull();
  });

  it.each([
    [['rust'], '0% · 0/2 skills', '-100%'],
    [['GO', 'sql'], '100% · 2/2 skills', '0%'],
  ])('renders coverage endpoints for %j', async (skills, label, offset) => {
    const ui = mount({ posting: { ...job, skills: ['go', 'sql'] } });
    await ui.tick();
    ui.answer(ui.calls()[0], { structuredContent: { skills }, content: [] });
    await ui.tick();
    expect(ui.window.document.querySelector('.match-text').textContent).toBe(label);
    expect(ui.window.document.querySelector('.match-fill').style.transform).toBe('translateX(' + offset + ')');
    ui.toolResult({ structuredContent: { jobs: [{ ...job, skills: ['go'] }] } });
    await ui.tick();
    expect(ui.calls()).toHaveLength(1);
    expect(ui.window.document.querySelector('.match-text').textContent).toBe(
      skills.includes('rust') ? '0% · 0/1 skills' : '100% · 1/1 skills',
    );
  });

  it.each([{ isError: true }, { structuredContent: { skills: [] } }, { content: [] }])(
    'omits match when a profile is unavailable: %j',
    async (reply) => {
      const ui = mount({ posting: { ...job, skills: ['go'] } });
      await ui.tick();
      ui.answer(ui.calls()[0], reply);
      await ui.tick();
      expect(ui.window.document.querySelector('.match')).toBeNull();
      expect(ui.window.document.body.textContent).toContain(job.title);
    },
  );

  it.each([[], undefined, [null, 42, {}, ''], ['go', null, 42, '']])(
    'handles missing or malformed job skills after loading a profile: %j',
    async (skills) => {
      const ui = mount({ posting: { ...job, skills: ['go'] } });
      await ui.tick();
      ui.answer(ui.calls()[0], { structuredContent: { skills: ['go'] }, content: [] });
      await ui.tick();
      ui.toolResult({ structuredContent: { jobs: [{ ...job, skills }] } });
      await ui.tick();
      expect(ui.window.document.querySelector('.title').textContent).toBe(job.title);
      if (skills?.includes('go')) {
        expect(ui.window.document.querySelector('.match-text').textContent).toBe('100% · 1/1 skills');
      } else {
        expect(ui.window.document.querySelector('.match')).toBeNull();
      }
      expect(ui.calls()).toHaveLength(1);
    },
  );

  it.each(['tool-error', 'rpc-error', 'timeout', 'malformed-json', 'missing-skills'])(
    'retries a failed profile read on a later result: %s',
    async (failure) => {
      const posting = { ...job, skills: ['go'] };
      const ui = mount({ posting });
      await ui.tick();
      const first = ui.calls()[0];
      ui.toolResult({ structuredContent: { jobs: [posting] } });
      expect(ui.calls()).toHaveLength(1);
      if (failure === 'timeout') ui.timers.find((timer) => timer.delay === 30000).callback();
      else if (failure === 'rpc-error')
        ui.receive({ jsonrpc: '2.0', id: first.id, error: { code: -32603, message: 'Temporary failure' } });
      else
        ui.answer(
          first,
          failure === 'tool-error'
            ? { isError: true, content: [] }
            : failure === 'malformed-json'
              ? { content: [{ type: 'text', text: 'invalid JSON' }] }
              : { structuredContent: {} },
        );
      await ui.tick();
      expect(ui.window.document.querySelector('.match')).toBeNull();
      expect(ui.calls()).toHaveLength(1);
      ui.toolResult({ structuredContent: { jobs: [posting] } });
      expect(ui.calls()).toHaveLength(2);
      ui.answer(ui.calls()[1], { structuredContent: { skills: ['go'] }, content: [] });
      await ui.tick();
      expect(ui.window.document.querySelector('.match-text').textContent).toBe('100% · 1/1 skills');
      ui.toolResult({ structuredContent: { jobs: [posting] } });
      expect(ui.calls()).toHaveLength(2);
    },
  );

  it('keeps a known empty profile cached across later results', async () => {
    const posting = { ...job, skills: ['go'] };
    const ui = mount({ posting });
    await ui.tick();
    ui.answer(ui.calls()[0], { structuredContent: { skills: [] }, content: [] });
    await ui.tick();
    ui.toolResult({ structuredContent: { jobs: [posting] } });
    await ui.tick();
    expect(ui.calls()).toHaveLength(1);
    expect(ui.window.document.querySelector('.match')).toBeNull();
  });

  it('adds profile coverage without replacing focused controls or pending-save status', async () => {
    const ui = mount({ posting: { ...job, skills: ['go'] } });
    await ui.tick();
    const profileCall = ui.calls()[0];
    const save = ui.button('Save job');
    const status = ui.window.document.querySelector('[role="status"]');
    save.click();
    const link = ui.window.document.querySelector('a');
    link.focus();
    ui.answer(profileCall, { structuredContent: { skills: ['go'] }, content: [] });
    await ui.tick();
    expect(ui.window.document.querySelector('.match-text').textContent).toBe('100% · 1/1 skills');
    expect(ui.window.document.activeElement).toBe(link);
    expect(ui.button('Saving...')).toBe(save);
    expect(ui.window.document.querySelector('[role="status"]')).toBe(status);
    expect(status.textContent).toContain('Saving');
    expect(save.disabled).toBe(true);
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
    ui.toolResult({ structuredContent: { jobs: [job], total: 1 } });
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
    ui.toolResult({ structuredContent: { jobs: [job], total: 1 } });
    await ui.tick();
    expect(ui.button('Saving...').disabled).toBe(true);
    ui.button('Saving...').click();
    expect(ui.calls()).toHaveLength(1);
    ui.answer(ui.calls()[0], { structuredContent: { job_id: 42, saved_at: '2026-11-05T03:00:00Z' }, content: [] });
    await ui.tick();
    expect(ui.button('Saved')?.disabled).toBe(true);
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
