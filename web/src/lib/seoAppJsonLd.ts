// schema.org JSON-LD for the site's own products (the API, the CLI, the browser
// extension) and its blog; the job and company structured data is in seo.ts.

import type { PostMeta } from './blog';
import { SITE } from './seo';

/** schema.org WebAPI for the API reference — names the endpoint set as one entity
 *  and points at the OpenAPI document, so an assistant can fetch the machine-readable
 *  spec instead of scraping the prose it is rendered beside. */
export function webApiJsonLd(origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebAPI',
    name: 'freehire API',
    description:
      'A read-first, open HTTP API over the freehire job catalogue: query jobs by seniority, skills, region and salary, read companies, and track applications with an API key.',
    url: `${origin}/docs/api`,
    documentation: `${origin}/docs/api`,
    termsOfService: `${origin}/privacy`,
    provider: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    // The OpenAPI document is the spec itself, not more prose about it.
    potentialAction: {
      '@type': 'ConsumeAction',
      target: { '@type': 'EntryPoint', urlTemplate: `${origin}/openapi.yaml`, encodingType: 'application/yaml' },
    },
  };
}

/** schema.org SoftwareApplication for the CLI page. Free and open source, so the
 *  zero-price `offers` is a fact rather than a marketing claim; `softwareHelp`
 *  points back at the page that documents it. */
export function cliApplicationJsonLd(origin: string, repositories: string[]): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: 'freehire CLI',
    description:
      'A small Go CLI and an MCP server over the freehire job API, so an AI agent or a script can search, open and track jobs without a browser. The hosted MCP connection signs in with OAuth.',
    url: `${origin}/cli`,
    applicationCategory: 'DeveloperApplication',
    operatingSystem: 'macOS, Linux',
    isAccessibleForFree: true,
    offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
    softwareHelp: { '@type': 'CreativeWork', url: `${origin}/cli` },
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    codeRepository: repositories,
  };
}

/** schema.org SoftwareApplication for the browser-extension landing. `installUrl`
 *  is passed in rather than written here so it comes from the same constant the
 *  page's buttons use (`extensionLinks.ts`) — structured data must not name a
 *  destination the page does not offer. One URL per store the page links. */
export function extensionApplicationJsonLd(origin: string, installUrls: string[]): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: 'freehire browser extension',
    description:
      "A job-application agent in your browser's side panel: it reads the posting you are on, scores it against your CV, and fills the application form from your profile — you press Submit.",
    url: `${origin}/features/extension`,
    applicationCategory: 'BrowserApplication',
    operatingSystem: 'Chrome, Firefox',
    browserRequirements: 'Requires Chrome 114, a Chromium browser with side-panel support, or Firefox',
    installUrl: installUrls,
    isAccessibleForFree: true,
    offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
    softwareHelp: { '@type': 'CreativeWork', url: `${origin}/features/extension` },
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
  };
}

/** schema.org Blog for the feed index, listing each post as a `BlogPosting`. A
 *  `Blog` (rather than a bare CollectionPage) is what ties the index to the
 *  `Article` on each post page, so engines read the feed as one publication.
 *  Summary entries only — the post bodies live on their own pages. */
export function blogJsonLd(posts: PostMeta[], origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'Blog',
    name: `${SITE} blog`,
    url: `${origin}/blog`,
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
    blogPost: posts.map((post) => ({
      '@type': 'BlogPosting',
      headline: post.title,
      description: post.summary,
      datePublished: post.date,
      url: `${origin}/blog/${post.slug}`,
    })),
  };
}

/** schema.org Article for a blog post page. Built from the validated `PostMeta`,
 *  so every field is present; `keywords` is emitted only when the post has tags. */
export function articleJsonLd(post: PostMeta, origin: string): Record<string, unknown> {
  const ld: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'Article',
    headline: post.title,
    description: post.summary,
    datePublished: post.date,
    url: `${origin}/blog/${post.slug}`,
    publisher: { '@type': 'Organization', name: SITE, url: `${origin}/` },
  };
  if (post.tags.length > 0) ld.keywords = post.tags.join(', ');
  return ld;
}
