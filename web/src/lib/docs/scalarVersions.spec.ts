import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// /docs/api is rendered twice: on the server by @scalar/server-side-rendering, which brings
// its OWN pinned @scalar/api-reference, and in the browser by the @scalar/api-reference this
// package depends on, whose stylesheet the page loads. Vue scopes Scalar's styles by a hash
// of each component, so two versions disagree on every hash: the server markup matches none
// of the loaded rules and the page renders as one unstyled column until the client remounts.
// It shipped that way for two weeks after Dependabot bumped one package without the other.
// Read from disk: neither package exports its package.json, so require.resolve refuses it.
const manifest = (name: string) =>
  JSON.parse(readFileSync(join(process.cwd(), 'node_modules', name, 'package.json'), 'utf8')) as {
    version: string;
    dependencies?: Record<string, string>;
  };

describe('Scalar packages', () => {
  it('render /docs/api on the server with the same api-reference the browser hydrates with', () => {
    const ssr = manifest('@scalar/server-side-rendering');
    expect(ssr.dependencies?.['@scalar/api-reference']).toBe(manifest('@scalar/api-reference').version);
  });
});
