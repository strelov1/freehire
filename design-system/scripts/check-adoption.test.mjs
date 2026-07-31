import { describe, expect, it } from 'vitest';
import { primitivesFrom, readImports } from './check-adoption.mjs';

describe('primitivesFrom', () => {
  // Derived from the package's own index rather than listed here, so a primitive
  // added to the export surface joins the census by itself. A hand-kept list of
  // fifteen names would drift exactly the way the DSDS props and the Storybook
  // argTypes already have.
  it('reads the default-exported components', () => {
    const index = `
      export { cn } from './cn.js';
      export { default as Alert } from './alert.svelte';
      export { default as Button } from './button.svelte';
    `;

    expect(primitivesFrom(index)).toEqual(['Alert', 'Button']);
  });

  it('leaves the variant helpers and types out', () => {
    const index = `
      export { default as Button } from './button.svelte';
      export { buttonVariants, type ButtonVariant, type ButtonSize } from './button.svelte';
    `;

    expect(primitivesFrom(index)).toEqual(['Button']);
  });
});

describe('readImports', () => {
  it('reads a single-line specifier list', () => {
    const { door } = readImports(`import { Button } from '$lib/ui';`);

    expect(door).toEqual(['Button']);
  });

  it('reads a multi-line specifier list', () => {
    const { door } = readImports(`
      import {
        Button,
        Badge,
      } from '$lib/ui';
    `);

    expect(door).toEqual(['Button', 'Badge']);
  });

  // The census asks which primitive a file reaches for, and renaming it locally
  // does not change that.
  it('records the imported name, not the local alias', () => {
    const { door } = readImports(`import { Button as PrimaryButton } from '$lib/ui';`);

    expect(door).toEqual(['Button']);
  });

  it('strips an inline type specifier', () => {
    const { door } = readImports(`import { type ButtonVariant, Button } from '$lib/ui';`);

    expect(door).toEqual(['ButtonVariant', 'Button']);
  });

  it('collects every statement in the file', () => {
    const { door } = readImports(`
      import { Button } from '$lib/ui';
      import { cn } from '$lib/ui';
    `);

    expect(door).toEqual(['Button', 'cn']);
  });

  // A primitive someone stopped using but left in a comment is not adoption.
  // check-token-coverage learned the same lesson: comments describe violations
  // as often as they commit them.
  it('does not count a commented-out import', () => {
    const { door } = readImports(`
      // import { Dialog } from '$lib/ui';
      /* import { Table } from '$lib/ui'; */
      import { Button } from '$lib/ui';
    `);

    expect(door).toEqual(['Button']);
  });

  it('ignores imports from anywhere else', () => {
    const { door } = readImports(`
      import { onMount } from 'svelte';
      import { api } from '$lib/api';
    `);

    expect(door).toEqual([]);
  });

  // The door rule. $lib/ui exists so that app code never names the package, and
  // there are zero of these today — so this one is not ratcheted, it is a wall.
  it('reports a file that names the package directly', () => {
    const { direct } = readImports(`import { Button } from 'freehire-design-system';`);

    expect(direct).toEqual(['freehire-design-system']);
  });

  it('reports a deep import into the package too', () => {
    const { direct } = readImports(`import Button from 'freehire-design-system/button.svelte';`);

    expect(direct).toEqual(['freehire-design-system/button.svelte']);
  });

  // web/src/app.css imports the package's theme.css, and must — it is the CSS
  // contract every consumer is required to import. The walk reads `from`
  // clauses, so a stylesheet's @import is never in its way.
  it('does not read a CSS @import as a module import', () => {
    const { door, direct } = readImports(`@import "freehire-design-system/theme.css";`);

    expect(door).toEqual([]);
    expect(direct).toEqual([]);
  });
});
