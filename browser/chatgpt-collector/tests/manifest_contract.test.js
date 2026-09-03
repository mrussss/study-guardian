import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const extensionRoot = join(root, '..');

test('manifest loads classic bundled Content Script and module background separately', () => {
  const manifest = JSON.parse(readFileSync(join(extensionRoot, 'manifest.json'), 'utf8'));
  assert.deepEqual(manifest.content_scripts?.[0]?.js, ['dist/content.js']);
  assert.equal(manifest.background?.service_worker, 'src/background.js');
  assert.equal(manifest.background?.type, 'module');
  assert.notEqual(manifest.content_scripts?.[0]?.js?.[0], 'src/content.js');
});
