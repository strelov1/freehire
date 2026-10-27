/** The freehire iPhone app's App Store listing — the only push-notification receiver.
 *  Read by the footer's store button and the onboarding tour's push channel.
 *
 *  NO country segment, on purpose. The listing's own share link is `/br/app/...`, which
 *  pins every visitor to the Brazilian storefront; without one Apple redirects each visitor
 *  to their own (measured: the bare form answers 301 to `/us/app/...`, the `/br/` form 200).
 *  A wrong-country page still loads, so nothing downstream could catch it. */
export const APP_STORE_URL = 'https://apps.apple.com/app/freehire-open-job-search/id6801885119';
