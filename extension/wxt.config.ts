import { defineConfig, type UserManifest } from 'wxt';
import tailwindcss from '@tailwindcss/vite';

/**
 * Pins a deterministic extension id, which is what makes the
 * `https://<id>.chromiumapp.org` redirect stable — hire allowlists that id, and
 * `launchWebAuthFlow` fails with "Authorization page could not be loaded" for
 * one it has never heard of.
 *
 * Only the store artifact drops it: the store assigns the id from its own key
 * and a mismatched `key` would fight it. Every other build keeps it, INCLUDING a
 * production `wxt build` — that is the artifact loaded unpacked for testing, and
 * an unpacked extension with no key takes its id from the directory path
 * instead. So the release is marked explicitly rather than inferred from `mode`,
 * which cannot tell the two `wxt build` outputs apart.
 *
 * The published id is a different one, and hire has to allowlist it as well.
 */
const RELEASE = process.env.WXT_RELEASE === '1';

const DEV_KEY =
  'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2GECbcIeOfNjo0xv8Q4bMKgLHN5qKCOQSYfVr3aTTHlIk1VliYT/Kb+xZiCyR34tBHFYlTaJATBWLIqmoTqQghXPMvM2LmNPo6ttNKuRdiNJNFvMz5M8RE7Qfgawj8EJo2dCTzWbSdRet2HLTKTBDLsZ/9F4RpXnDQrWVaqQqDvNO9CXT99UqBuzpuTXq3WyWJYcnSjDETND62jkW2kUKj0YpybpxMYDjC1NwoXGgI2f6z3gy2FB/ld3yiaYwVu2NwZGTILgmKLQKGrutHQktnVc27qoeBVvO0s8VMnJkf3kFJPPNmJktGAbnvdiGlx4wBEGBqVEqFBhBaj0GlE1dwIDAQAB';

export default defineConfig({
  modules: ['@wxt-dev/module-svelte'],
  vite: () => ({ plugins: [tailwindcss()] }),
  manifest: () =>
    ({
      ...(RELEASE ? {} : { key: DEV_KEY }),
      name: 'freehire',
      description: 'Your job-application agent, on any page.',
      permissions: ['storage', 'tabs', 'sidePanel', 'scripting', 'activeTab', 'identity'],
      host_permissions: ['<all_urls>'],
      // `icons` is discovered from public/icon/*.png. The variants swap in the
      // inverted mark on a dark toolbar.
      //
      // Store artifact only. `icon_variants` is still Canary-gated, and stable
      // Chrome does NOT quietly ignore it — loading an unpacked build carrying the
      // key reports "'icon_variants' requires canary channel or newer", which is
      // noise on every local install and unclear enough to read as a real failure.
      // The store's own review runs on a channel that understands it, and every
      // Chrome without it falls back to `icons`.
      //
      // Both groups name their scheme: a group without `color_schemes` is a
      // wildcard, and the first match by array order wins — so a wildcard here
      // would shadow the dark one.
      ...(RELEASE
        ? {
            icon_variants: [
              {
                16: 'icon/16.png',
                32: 'icon/32.png',
                48: 'icon/48.png',
                128: 'icon/128.png',
                color_schemes: ['light'],
              },
              {
                16: 'icon/dark/16.png',
                32: 'icon/dark/32.png',
                48: 'icon/dark/48.png',
                128: 'icon/dark/128.png',
                color_schemes: ['dark'],
              },
            ],
          }
        : {}),
      side_panel: {
        default_path: 'sidepanel.html',
      },
      action: {},
    }) as UserManifest,
});
