import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const extensionRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = join(extensionRoot, '..', '..');

test('Content Script bundler emits a classic script without static imports', () => {
  const temp = mkdtempSync(join(tmpdir(), 'studyguardian-bundle-'));
  try {
    const output = join(temp, 'content.js');
    const python = process.platform === 'win32' ? 'python' : 'python3';
    execFileSync(python, [join(repoRoot, 'scripts', 'bundle-content.py'), join(extensionRoot, 'src', 'content.js'), output], { stdio: 'pipe' });
    const bundle = readFileSync(output, 'utf8');
    assert.ok(bundle.includes('function parseConversation'));
    assert.ok(bundle.includes('class MessageStateTracker'));
    assert.doesNotMatch(bundle, /^\s*import\s/m);
    assert.doesNotMatch(bundle, /^\s*export\s/m);
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});
