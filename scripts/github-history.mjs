import { readFileSync, existsSync, mkdirSync, writeFileSync, renameSync, unlinkSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';

const repository = 'aklkbqx/agy-swap';
const versionPattern = /^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;

function isPublicUrl(value) {
  if (typeof value !== 'string') return false;
  try { return new URL(value).protocol === 'https:'; } catch { return false; }
}

export function readPages(text) {
  const data = JSON.parse(text);
  if (!Array.isArray(data)) throw new Error('GitHub pages must be arrays');
  if (!data.some(Array.isArray)) return data;
  if (!data.every(Array.isArray)) throw new Error('Mixed GitHub page arrays');
  return data.flat();
}

function tagsByName(tags, surface) {
  if (!Array.isArray(tags)) throw new Error(`${surface} tags must be an array`);
  const result = new Map();
  for (const t of tags) {
    if (!t || typeof t.name !== 'string' || !/^[0-9a-f]{40}$/i.test(t.commit?.sha ?? '')) {
      throw new Error(`Invalid ${surface} tag name or commit`);
    }
    if (result.has(t.name) && result.get(t.name).commit.sha !== t.commit.sha) {
      throw new Error(`Conflicting duplicate ${surface} tag: ${t.name}`);
    }
    result.set(t.name, t);
  }
  return result;
}

function compareTags(a, b) {
  const parts = name => name.replace(/^v/, '').split('+')[0].split('-');
  const [aa, ...ap] = parts(a), [bb, ...bp] = parts(b);
  const an = aa.split('.').map(Number), bn = bb.split('.').map(Number);
  for (let i = 0; i < 3; i++) if (an[i] !== bn[i]) return bn[i] - an[i];
  if (!ap.length && bp.length) return -1;
  if (ap.length && !bp.length) return 1;
  return b.localeCompare(a, 'en', { numeric: true });
}

export function buildInventory({ tags, releases, siteTags, latestStable }) {
  const tool = tagsByName(tags, 'Tool'), site = tagsByName(siteTags, 'Site');
  if (!Array.isArray(releases)) throw new Error('Releases must be an array');
  const byRelease = new Map();
  for (const r of releases) {
    if (!r || typeof r.tag_name !== 'string' || typeof r.draft !== 'boolean' || typeof r.prerelease !== 'boolean'
      || !Array.isArray(r.assets)) throw new Error('Invalid release metadata');
    if (r.published_at != null && (typeof r.published_at !== 'string' || Number.isNaN(Date.parse(r.published_at)))) {
      throw new Error('Invalid publication date');
    }
    if (r.html_url != null && !isPublicUrl(r.html_url)) throw new Error('Invalid public release URL');
    if (byRelease.has(r.tag_name)) throw new Error(`Duplicate release: ${r.tag_name}`);
    if (!r.draft && !tool.has(r.tag_name)) throw new Error(`Release missing Tool tag: ${r.tag_name}`);
    byRelease.set(r.tag_name, r);
  }
  const latest = byRelease.get(latestStable?.tag_name);
  if (!latest || latest.draft || latest.prerelease || !tool.has(latest.tag_name)) {
    throw new Error('Latest stable release must have a published stable Release and Tool tag');
  }
  const record = t => {
    const r = byRelease.get(t.name), published = r && !r.draft;
    return {
      tag: t.name, toolCommit: t.commit.sha,
      tagUrl: `https://github.com/${repository}/tree/${encodeURIComponent(t.name)}`,
      releaseState: !r ? 'tag-only' : r.draft ? 'draft' : r.prerelease ? 'prerelease' : 'published',
      prerelease: r ? r.prerelease : null,
      publishedAt: published ? r.published_at ?? null : null,
      releaseUrl: published ? r.html_url ?? `https://github.com/${repository}/releases/tag/${encodeURIComponent(t.name)}` : null,
      assets: published ? r.assets.map(a => {
        if (!a || typeof a.name !== 'string' || !Number.isSafeInteger(a.size) || a.size < 0
          || !isPublicUrl(a.browser_download_url) || (a.digest != null && typeof a.digest !== 'string')) {
          throw new Error('Invalid public asset metadata');
        }
        return { name: a.name, size: a.size, url: a.browser_download_url, digest: a.digest ?? null };
      }).sort((a, b) => a.name.localeCompare(b.name, 'en')) : [],
      siteTagPresent: site.has(t.name), evidence: 'historical-metadata', evidenceDocument: null,
    };
  };
  const all = [...tool.values()];
  return {
    schemaVersion: 1, repository, latestStableTag: latest.tag_name,
    siteTagObservation: 'Complete private repository tags API read; public output contains tag-presence only.',
    versions: all.filter(t => versionPattern.test(t.name)).sort((a, b) => compareTags(a.name, b.name)).map(record),
    otherTags: all.filter(t => !versionPattern.test(t.name)).sort((a, b) => a.name.localeCompare(b.name, 'en')).map(record),
  };
}

export function renderIndex(inventory) {
  const released = inventory.versions.filter(v => v.releaseState === 'published').length;
  const tagOnly = inventory.versions.filter(v => v.releaseState === 'tag-only').length;
  const lines = ['# Release history', '',
    `All ${inventory.versions.length} observed Tool version tags are recorded: ${released} published stable Releases and ${tagOnly} tag-only versions. Latest stable: **${inventory.latestStableTag}**.`, '',
    'The [JSON inventory](index.json) contains public Tool commits and actual asset metadata. Historical metadata is not retrospective installation/deployment verification. Missing Site tags mean no matching tag was observed, not that no deployment occurred.', '',
    'Historical documentation covers all versions; security fixes support only latest stable. Existing tags and Release bodies are preserved.', '',
    '| Version | GitHub record | Published UTC | Assets | Matching Site tag observed | Evidence |',
    '| --- | --- | --- | ---: | --- | --- |'];
  for (const v of inventory.versions) {
    const label = v.releaseState === 'tag-only' ? 'Tag only' : v.releaseState;
    const evidence = v.evidenceDocument ? `[Delivery summary](${v.evidenceDocument})` : 'Recorded metadata';
    lines.push(`| ${v.tag} | [${label}](${v.releaseUrl ?? v.tagUrl}) | ${v.publishedAt ?? 'Unknown; not published'} | ${v.assets.length} | ${v.siteTagPresent ? 'Yes' : 'No'} | ${evidence} |`);
  }
  if (inventory.otherTags.length) {
    lines.push('', '## Other tags', '');
    for (const v of inventory.otherTags) lines.push(`- [${v.tag}](${v.tagUrl})`);
  }
  lines.push('', '## Refresh', '',
    'From the Tool repository, run `node scripts/github-history.mjs --write`, then `node scripts/github-history.mjs --check`. Reads use paginated GitHub APIs; writes affect this generated index only. Use `--input` with a reviewed JSON fixture for offline verification. The helper does not publish source, change Issues/tags/Releases or overwrite human version evidence.', '',
    'Missing Site access or invalid history fails before output changes. Keep version evidence in separate `vX.Y.Z.md` files. New records begin as historical metadata; promote evidence only after checks have actually been recorded.', '');
  return lines.join('\n');
}

export function updateHistory(directory, inventory, { check = false } = {}) {
  const jsonPath = join(directory, 'index.json'), markdownPath = join(directory, 'README.md');
  let old;
  if (existsSync(jsonPath)) {
    old = JSON.parse(readFileSync(jsonPath, 'utf8'));
    if (old.schemaVersion !== 1) throw new Error('Unsupported history schema; explicit migration required');
  }
  // Human evidence classification survives refreshes; no version document is written here.
  const existing = new Map((old?.versions ?? []).map(v => [v.tag, v]));
  const next = { ...inventory, versions: inventory.versions.map(v => {
    const previous = existing.get(v.tag);
    if (previous && previous.toolCommit !== v.toolCommit) throw new Error(`Immutable tag commit changed: ${v.tag}`);
    return { ...v, evidence: previous?.evidence ?? v.evidence,
      evidenceDocument: previous?.evidenceDocument ?? v.evidenceDocument };
  }) };
  if (old?.snapshotDate) next.snapshotDate = old.snapshotDate;
  if (old?.versions?.some(v => !next.versions.some(n => n.tag === v.tag))) {
    throw new Error('Previously recorded version tag missing from fresh inventory');
  }
  const json = JSON.stringify(next, null, 2) + '\n', markdown = renderIndex(next);
  const changed = !existsSync(jsonPath) || readFileSync(jsonPath, 'utf8') !== json
    || !existsSync(markdownPath) || readFileSync(markdownPath, 'utf8') !== markdown;
  if (check || !changed) return { changed };
  mkdirSync(directory, { recursive: true });
  // Validate and stage both outputs before replacing either. Each replacement is atomic.
  const temporary = [jsonPath, markdownPath].map(p => `${p}.${randomUUID()}.tmp`);
  try {
    writeFileSync(temporary[0], json, { flag: 'wx' });
    writeFileSync(temporary[1], markdown, { flag: 'wx' });
    renameSync(temporary[0], jsonPath);
    renameSync(temporary[1], markdownPath);
  } finally {
    for (const p of temporary) if (existsSync(p)) unlinkSync(p);
  }
  return { changed };
}

function fetchGitHub(endpoint, paginated = true) {
  const args = ['api', endpoint, ...(paginated ? ['--paginate', '--slurp'] : [])];
  const result = spawnSync('gh', args, { encoding: 'utf8', timeout: 120000, maxBuffer: 32 * 1024 * 1024 });
  if (result.error || result.status !== 0) throw new Error(`GitHub read failed for ${endpoint}; check gh access. Output unchanged.`);
  return paginated ? readPages(result.stdout) : JSON.parse(result.stdout);
}

function run(args) {
  let check = false, write = false, input, output = resolve(dirname(fileURLToPath(import.meta.url)), '../docs/releases');
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '--check') check = true;
    else if (arg === '--write') write = true;
    else if (arg === '--input' || arg === '--output') {
      if (!args[i + 1] || args[i + 1].startsWith('--')) throw new Error(`Missing value for ${arg}`);
      if (arg === '--input') input = args[++i]; else output = resolve(args[++i]);
    } else throw new Error(`Unknown option: ${arg}`);
  }
  if (check === write) throw new Error('Select exactly one mode: --check or --write');
  const data = input ? JSON.parse(readFileSync(input, 'utf8')) : {
    tags: fetchGitHub(`repos/${repository}/tags`),
    releases: fetchGitHub(`repos/${repository}/releases`),
    siteTags: fetchGitHub('repos/aklkbqx/agy-swap-site/tags'),
    latestStable: fetchGitHub(`repos/${repository}/releases/latest`, false),
  };
  const inventory = buildInventory(data);
  const result = updateHistory(output, inventory, { check });
  process.stdout.write(JSON.stringify({ mode: check ? 'check' : 'write', changed: result.changed,
    versions: inventory.versions.length, latestStableTag: inventory.latestStableTag }) + '\n');
  return check && result.changed ? 1 : 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = run(process.argv.slice(2)); }
  catch (error) { process.stderr.write(`History refresh failed: ${error.message}\n`); process.exitCode = 1; }
}
