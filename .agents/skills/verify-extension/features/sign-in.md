# Install + sign in

The unpacked build connects to the owner's freehire.me account through
`chrome.identity.launchWebAuthFlow` and the consent page.

## Sub-features

- Toolbar icon opens the side panel.
- Sign in with freehire → consent page → panel signed in.
- Sign out (panel header).

## How to get to it (user POV)

Open any page, click the freehire icon in the toolbar, press "Sign in with freehire", press
"Connect" on the page that opens.

## Driving it with cua_*

1. On any tab, press toolbar `freehire\nHas access to this site`.
2. Panel (`AXWebArea "freehire"`) shows `Sign in with freehire` and chip `offline`; press it.
3. A window titled `The freehire extension wants to access your account` opens with
   `Signed in as <email>`; its URL carries
   `redirect_uri=https://ebhfafmnihndlaoibgjnfjhkjlnaencj.chromiumapp.org/`. Press `Connect`.
4. Proof: the window closes; panel header reads `Signed in as <email>`, chip `ready`.

## Gotchas

- The consent page says to allow only if you pressed Sign in yourself. Press Connect only
  when the owner asked for this run.
- The redirect id must be the pinned dev id; a build without `key` gets a path-derived id that
  hire has not allowlisted, and the window fails with "Authorization page could not be loaded".
