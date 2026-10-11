import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

// Feature missing is an assertion failure, rather than an unhandled import error.
const module = await import('./github-history.mjs').catch(() => ({}));
const { buildInventory, readPages, renderIndex, updateHistory } = module;
const tag = (name, sha = 'a'.repeat(40)) => ({ name, commit: { sha } });
const release = (name, extra = {}) => ({ tag_name: name, draft: false, prerelease: false,
  published_at: '2026-10-11T08:06:11Z', html_url: `https://github.com/aklkbqx/agy-swap/releases/tag/${name}`,
  assets: [], ...extra });
const input = (extra = {}) => ({ tags: [tag('v1.5.0'), tag('v2.11.2', 'b'.repeat(40))],
  releases: [release('v2.11.2')], siteTags: [tag('v1.5.0')], latestStable: { tag_name: 'v2.11.2' }, ...extra });
const temporary = (t) => {
  const path = mkdtempSync(join(tmpdir(), 'agy-history-'));
  t.after(() => rmSync(path, { recursive: true, force: true }));
  return path;
};

test('the history feature exposes the planned read-only interfaces', () => {
  for (const f of [buildInventory, readPages, renderIndex, updateHistory]) assert.equal(typeof f, 'function');
});

test('all API pages are retained and invalid pagination is rejected', () => {
  assert.equal(typeof readPages, 'function');
  assert.deepEqual(readPages(JSON.stringify([[tag('v1.5.0')], [tag('v2.11.2')]])),
    [tag('v1.5.0'), tag('v2.11.2')]);
  assert.deepEqual(readPages(JSON.stringify([tag('v1.5.0')])), [tag('v1.5.0')]);
  assert.throws(() => readPages('{broken'), /JSON|Unexpected|Expected/i);
  assert.throws(() => readPages('[[],{}]'), /page|array/i);
});

test('tag-only versions retain null dates, no assets and explicit Site evidence', () => {
  assert.equal(typeof buildInventory, 'function');
  const x = buildInventory(input());
  const old = x.versions.find(v => v.tag === 'v1.5.0');
  assert.equal(old.releaseState, 'tag-only');
  assert.equal(old.publishedAt, null);
  assert.equal(old.releaseUrl, null);
  assert.deepEqual(old.assets, []);
  assert.equal(old.siteTagPresent, true);
  assert.equal(x.versions.find(v => v.tag === 'v2.11.2').siteTagPresent, false);
});

test('historical asset shapes and null digests are preserved', () => {
  assert.equal(typeof buildInventory, 'function');
  const asset = { name: 'python-source.zip', size: 123, browser_download_url: 'https://example.com/python-source.zip' };
  const x = buildInventory(input({ releases: [release('v2.11.2', { assets: [asset] })] }));
  assert.deepEqual(x.versions[0].assets, [{ name: asset.name, size: 123, url: asset.browser_download_url, digest: null }]);
});

test('draft and prerelease observations never masquerade as stable releases', () => {
  assert.equal(typeof buildInventory, 'function');
  const x = buildInventory(input({ tags: [...input().tags, tag('v2.12.0-rc.1'), tag('v2.12.0')],
    releases: [...input().releases, release('v2.12.0-rc.1', { prerelease: true }), release('v2.12.0', { draft: true })] }));
  assert.equal(x.versions.find(v => v.tag === 'v2.12.0-rc.1').releaseState, 'prerelease');
  const draft = x.versions.find(v => v.tag === 'v2.12.0');
  assert.equal(draft.releaseState, 'draft');
  assert.equal(draft.publishedAt, null);
  assert.equal(draft.releaseUrl, null);
  assert.deepEqual(draft.assets, []);
  assert.equal(x.latestStableTag, 'v2.11.2');
});

test('conflicting tags, absent latest stable and release/tag mismatches are rejected', () => {
  assert.equal(typeof buildInventory, 'function');
  assert.throws(() => buildInventory(input({ tags: [...input().tags, tag('v1.5.0', 'c'.repeat(40))] })), /duplicate|conflict/i);
  assert.throws(() => buildInventory(input({ latestStable: { tag_name: 'v9.0.0' } })), /latest|tag/i);
  assert.throws(() => buildInventory(input({ releases: [release('v2.11.2', { prerelease: true })] })), /stable/i);
  assert.throws(() => buildInventory(input({ releases: [...input().releases, release('v9.0.0')] })), /missing.*tag|tag.*missing/i);
  assert.throws(() => buildInventory(input({ tags: [tag('v1.5.0', 'bad'), input().tags[1]] })), /commit/i);
});

test('non-version tags are recorded, duplicate same-SHA observations are idempotent', () => {
  assert.equal(typeof buildInventory, 'function');
  const x = buildInventory(input({ tags: [...input().tags, tag('snapshot'), tag('v1.5.0')] }));
  assert.equal(x.versions.length, 2);
  assert.deepEqual(x.otherTags.map(v => v.tag), ['snapshot']);
});

test('numeric ordering and output are deterministic under shuffled API input', () => {
  assert.equal(typeof buildInventory, 'function');
  const a = input({ tags: [...input().tags, tag('v2.9.0'), tag('v2.10.0')] });
  const b = { ...a, tags: [...a.tags].reverse(), releases: [...a.releases].reverse() };
  assert.deepEqual(buildInventory(a), buildInventory(b));
  assert.deepEqual(buildInventory(a).versions.map(v => v.tag), ['v2.11.2', 'v2.10.0', 'v2.9.0', 'v1.5.0']);
  assert.equal(renderIndex(buildInventory(a)), renderIndex(buildInventory(b)));
});

test('check mode leaves stale output and human evidence untouched; writes then check without churn', () => {
  assert.equal(typeof updateHistory, 'function');
  const dir = mkdtempSync(join(tmpdir(), 'agy-history-'));
  try {
    writeFileSync(join(dir, 'index.json'), JSON.stringify({ schemaVersion: 1, ...buildInventory(input()) }));
    writeFileSync(join(dir, 'README.md'), 'old index\n');
    writeFileSync(join(dir, 'v2.11.2.md'), '# Human evidence\n');
    const before = readFileSync(join(dir, 'index.json'), 'utf8');
    assert.equal(updateHistory(dir, buildInventory(input()), { check: true }).changed, true);
    assert.equal(readFileSync(join(dir, 'index.json'), 'utf8'), before);
    updateHistory(dir, buildInventory(input()));
    const after = readFileSync(join(dir, 'index.json'), 'utf8');
    assert.equal(updateHistory(dir, buildInventory(input()), { check: true }).changed, false);
    updateHistory(dir, buildInventory(input()));
    assert.equal(readFileSync(join(dir, 'index.json'), 'utf8'), after);
    assert.equal(readFileSync(join(dir, 'v2.11.2.md'), 'utf8'), '# Human evidence\n');
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('unsupported existing schemas fail before replacing either output', t => {
  assert.equal(typeof updateHistory, 'function');
  const dir = temporary(t);
  writeFileSync(join(dir, 'index.json'), '{"schemaVersion":99}');
  writeFileSync(join(dir, 'README.md'), 'preserve me');
  assert.throws(() => updateHistory(dir, buildInventory(input())), /schema/i);
  assert.equal(readFileSync(join(dir, 'index.json'), 'utf8'), '{"schemaVersion":99}');
  assert.equal(readFileSync(join(dir, 'README.md'), 'utf8'), 'preserve me');
});

test('immutable version commits and removed history cannot be overwritten', t => {
  const dir = temporary(t), original = buildInventory(input());
  updateHistory(dir, original);
  const before = readFileSync(join(dir, 'index.json'), 'utf8');
  assert.throws(() => updateHistory(dir, buildInventory(input({ tags: [tag('v1.5.0', 'c'.repeat(40)), input().tags[1]] }))), /immutable|commit changed/i);
  assert.throws(() => updateHistory(dir, buildInventory(input({ tags: [input().tags[1]] }))), /missing/i);
  assert.equal(readFileSync(join(dir, 'index.json'), 'utf8'), before);
});

test('public history never emits private Site commits or draft asset URLs', () => {
  const privateSha = 'f'.repeat(40), privateUrl = 'https://example.com/private-draft.zip';
  const x = buildInventory(input({ siteTags: [tag('v1.5.0', privateSha)],
    tags: [...input().tags, tag('v2.12.0')], releases: [...input().releases,
      release('v2.12.0', { draft: true, assets: [{ name: 'draft.zip', size: 1, browser_download_url: privateUrl }] })] }));
  assert.equal(JSON.stringify(x).includes(privateSha), false);
  assert.equal(JSON.stringify(x).includes(privateUrl), false);
});

test('CLI fixture mode is offline; malformed input and stale checks do not mutate output', t => {
  const dir = temporary(t), fixtures = join(dir, 'input.json'), out = join(dir, 'releases');
  writeFileSync(fixtures, JSON.stringify(input()));
  const command = (args) => spawnSync(process.execPath, [resolve('scripts/github-history.mjs'), ...args], { encoding: 'utf8' });
  const first = command(['--write', '--input', fixtures, '--output', out]);
  assert.equal(first.status, 0, first.stderr);
  const before = readFileSync(join(out, 'index.json'), 'utf8');
  assert.equal(command(['--check', '--input', fixtures, '--output', out]).status, 0);
  writeFileSync(fixtures, '{broken');
  assert.notEqual(command(['--write', '--input', fixtures, '--output', out]).status, 0);
  assert.equal(readFileSync(join(out, 'index.json'), 'utf8'), before);
  const empty = join(dir, 'absent');
  writeFileSync(fixtures, JSON.stringify(input()));
  assert.equal(command(['--check', '--input', fixtures, '--output', empty]).status, 1);
  assert.equal(existsSync(empty), false);
});

test('CLI GitHub read failure preserves existing output without disclosure', t => {
  const dir = temporary(t), out = join(dir, 'releases');
  // An empty PATH makes the real gh executable unavailable: no fake service or token needed.
  const run = spawnSync(process.execPath, [resolve('scripts/github-history.mjs'), '--write', '--output', out],
    { encoding: 'utf8', env: { ...process.env, PATH: '' } });
  assert.notEqual(run.status, 0);
  assert.equal(existsSync(out), false);
  assert.match(run.stderr, /gh|GitHub/i);
});

test('malformed publication fields fail before either history file is replaced', t => {
  const dir = temporary(t), fixture = join(dir, 'input.json'), out = join(dir, 'releases');
  updateHistory(out, buildInventory(input()));
  const before = ['index.json', 'README.md'].map(name => readFileSync(join(out, name), 'utf8'));
  for (const fields of [{ published_at: {} }, { published_at: 'not-a-date' },
    { html_url: 7 }, { html_url: 'javascript:alert(1)' },
    { assets: [{ name: 'binary', size: 1, browser_download_url: 'https://example.com/binary', digest: {} }] }]) {
    writeFileSync(fixture, JSON.stringify(input({ releases: [release('v2.11.2', fields)] })));
    const run = spawnSync(process.execPath, [resolve('scripts/github-history.mjs'), '--write', '--input', fixture, '--output', out], { encoding: 'utf8' });
    assert.notEqual(run.status, 0, JSON.stringify(fields));
    assert.deepEqual(['index.json', 'README.md'].map(name => readFileSync(join(out, name), 'utf8')), before);
  }
});
