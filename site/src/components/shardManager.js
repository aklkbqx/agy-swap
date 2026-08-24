import { fetchShard } from './fixtures.js';

export function createShardManager(customFetch = fetchShard) {
  const fullCache = new Map();
  const inflight = new Map();
  let currentRequestId = 0;

  function hasFullShard(layout, view) {
    const key = `${layout}.${view.toLowerCase()}`;
    const cached = fullCache.get(key);
    return Array.isArray(cached) && cached.length > 0;
  }

  function getLoadedShard(layout, view) {
    const key = `${layout}.${view.toLowerCase()}`;
    const cached = fullCache.get(key);
    return Array.isArray(cached) && cached.length > 0 ? cached : null;
  }

  function ensureShard(layout, view) {
    const key = `${layout}.${view.toLowerCase()}`;
    const cached = fullCache.get(key);
    if (Array.isArray(cached) && cached.length > 0) {
      return Promise.resolve(cached);
    }
    if (inflight.has(key)) {
      return inflight.get(key);
    }
    const p = customFetch(layout, view)
      .then((fixtures) => {
        if (!Array.isArray(fixtures) || fixtures.length === 0) {
          throw new Error(`Failed to load valid non-empty fixtures for ${key}`);
        }
        fullCache.set(key, fixtures);
        inflight.delete(key);
        return fixtures;
      })
      .catch((err) => {
        inflight.delete(key);
        throw err;
      });
    inflight.set(key, p);
    return p;
  }

  function nextRequestId() {
    currentRequestId += 1;
    return currentRequestId;
  }

  function isCurrentRequest(id) {
    return id === currentRequestId;
  }

  return {
    hasFullShard,
    getLoadedShard,
    ensureShard,
    nextRequestId,
    isCurrentRequest,
  };
}
