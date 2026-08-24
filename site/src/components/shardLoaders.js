// This module is statically transformed by Vite at build time.
// import.meta.glob creates dynamic import chunks for each shard file.
export const shardLoaders = import.meta.glob('../generated/shards/*.json');

export async function loadBrowserShard(key) {
  const path = `../generated/shards/${key}.json`;
  const loader = shardLoaders[path];
  if (!loader) {
    throw new Error(`Unknown shard path: ${path}`);
  }
  const mod = await loader();
  const fixtures = mod.default?.fixtures || mod.fixtures;
  if (!Array.isArray(fixtures) || fixtures.length === 0) {
    throw new Error(`Shard ${key} contained no fixtures`);
  }
  return fixtures;
}
