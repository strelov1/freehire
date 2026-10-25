import { afterEach, describe, expect, it } from 'vitest';
import { CLI_BANNER_DISMISSED_COOKIE, readDismissed, writeDismissed } from './cliPromo';

// The node environment has no document at all, so each test installs a minimal cookie
// jar and removes it afterwards. Like a browser's, a write sets one cookie (its
// attributes dropped) and a read returns every cookie as "name=value; name=value".
function installCookies(jar: Map<string, string>) {
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: {
      get cookie() {
        return [...jar].map(([k, v]) => `${k}=${v}`).join('; ');
      },
      set cookie(raw: string) {
        const pair = raw.split(';')[0] ?? '';
        const eq = pair.indexOf('=');
        jar.set(pair.slice(0, eq).trim(), pair.slice(eq + 1));
      },
    },
  });
}

/** A blocked origin: every access throws. */
function installHostileCookies() {
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    get() {
      throw new Error('access denied');
    },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'document');
});

describe('dismissal', () => {
  it('round-trips through the cookie', () => {
    installCookies(new Map());

    expect(readDismissed()).toBe(false);
    writeDismissed();
    expect(readDismissed()).toBe(true);
  });

  it('is not fooled by a cookie whose name merely ends the same way', () => {
    installCookies(new Map([[`x${CLI_BANNER_DISMISSED_COOKIE}`, '1']]));

    expect(readDismissed()).toBe(false);
  });

  it('writes a session cookie under the name the app.html no-flash script reads', () => {
    let written = '';
    Object.defineProperty(globalThis, 'document', {
      configurable: true,
      value: {
        get cookie() {
          return '';
        },
        set cookie(raw: string) {
          written = raw;
        },
      },
    });

    writeDismissed();

    expect(CLI_BANNER_DISMISSED_COOKIE).toBe('hire_cli_banner_dismissed');
    expect(written.startsWith('hire_cli_banner_dismissed=1;')).toBe(true);
    // No lifetime: the dismissal ends with the browser session, and the strip asks again.
    expect(written).not.toMatch(/max-age|expires/i);
  });

  it('reads as not dismissed when cookies are unavailable', () => {
    installHostileCookies();

    expect(readDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileCookies();

    expect(() => writeDismissed()).not.toThrow();
  });
});
