// Validates that all DSDS JSON docs are well-formed and have the required fields.
// No external schema dependency — just structural checks.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const docsDir = join(import.meta.dirname, '..', 'docs', 'dsds');
let errors = 0;

for (const file of readdirSync(docsDir)) {
  if (!file.endsWith('.json')) continue;
  const path = join(docsDir, file);
  let data;
  try {
    data = JSON.parse(readFileSync(path, 'utf-8'));
  } catch (e) {
    console.error(`✗ ${file}: invalid JSON — ${e.message}`);
    errors++;
    continue;
  }
  if (!Array.isArray(data.entities)) {
    console.error(`✗ ${file}: missing "entities" array`);
    errors++;
    continue;
  }
  for (const entity of data.entities) {
    if (!entity.id) { console.error(`✗ ${file}: entity missing "id"`); errors++; }
    if (!entity.type) { console.error(`✗ ${file}: entity "${entity.id}" missing "type"`); errors++; }
    if (!entity.name) { console.error(`✗ ${file}: entity "${entity.id}" missing "name"`); errors++; }
  }
  console.log(`✓ ${file}: ${data.entities.length} entities valid`);
}

if (errors > 0) {
  console.error(`\n${errors} error(s) found.`);
  process.exit(1);
} else {
  console.log('\nAll DSDS docs valid.');
}
