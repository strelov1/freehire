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

/**
 * Firefox's add-on id. It is what Firefox hashes into the identity redirect,
 * `https://<sha1(id)>.extensions.allizom.org/`, so like Chrome's `key` it must
 * never change — hire allowlists that hash.
 */
const GECKO_ID = 'extension@freehire.me';

const DEV_KEY =
  'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2GECbcIeOfNjo0xv8Q4bMKgLHN5qKCOQSYfVr3aTTHlIk1VliYT/Kb+xZiCyR34tBHFYlTaJATBWLIqmoTqQghXPMvM2LmNPo6ttNKuRdiNJNFvMz5M8RE7Qfgawj8EJo2dCTzWbSdRet2HLTKTBDLsZ/9F4RpXnDQrWVaqQqDvNO9CXT99UqBuzpuTXq3WyWJYcnSjDETND62jkW2kUKj0YpybpxMYDjC1NwoXGgI2f6z3gy2FB/ld3yiaYwVu2NwZGTILgmKLQKGrutHQktnVc27qoeBVvO0s8VMnJkf3kFJPPNmJktGAbnvdiGlx4wBEGBqVEqFBhBaj0GlE1dwIDAQAB';

export default defineConfig({
  modules: ['@wxt-dev/module-svelte'],
  // The sources ZIP addons.mozilla.org reviews a bundled build against. It is
  // rooted at the repository because the bundle compiles in ../design-system
  // (a `file:` dependency), and names .env.production explicitly: hidden files
  // are skipped by default, and without it a reviewer's build points at localhost.
  zip: {
    // Keep hidden maps beside the private build, out of store artifacts.
    exclude: ['**/*.map'],
    sourcesRoot: '..',
    includeSources: [
      'extension/**',
      'extension/.env.production',
      'design-system/**',
    ],
  },
  vite: () => ({ plugins: [tailwindcss()], build: { sourcemap: 'hidden' } }),
  manifest: ({ browser }) =>
    ({
      // Firefox ignores `key` and pins its id through `gecko.id` instead.
      ...(RELEASE || browser === 'firefox' ? {} : { key: DEV_KEY }),
      ...(browser === 'firefox'
        ? {
            browser_specific_settings: {
              gecko: {
                id: GECKO_ID,
                // 140 is the first release that reads data_collection_permissions,
                // which addons.mozilla.org requires of every new listing.
                strict_min_version: '140.0',
                data_collection_permissions: {
                  required: ['authenticationInfo', 'websiteContent'],
                },
              },
            },
          }
        : {}),
      name: 'freehire',
      description: 'Your job-application agent, on any page.',
      // `debugger`/`downloads` back the "Attach tailored CV" action only: attaching the
      // debugger is what lets DOM.setFileInputFiles place a file into an
      // `input[type=file]`, since neither a page's own script nor a content script can
      // assign HTMLInputElement.files (see extension-attach-tailored-cv's design.md).
      //
      // Firefox has neither: no `chrome.debugger` (so no Attach tailored CV — the panel
      // hides it) and a sidebar instead of `sidePanel`, which WXT emits as
      // `sidebar_action` from the same entrypoint.
      permissions: [
        'storage',
        'tabs',
        'scripting',
        'activeTab',
        'identity',
        'downloads',
        ...(browser === 'firefox' ? [] : ['sidePanel', 'debugger']),
      ],
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
      // would shadow the dark one. Chrome's key only: Firefox warns on it.
      ...(RELEASE && browser !== 'firefox'
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
