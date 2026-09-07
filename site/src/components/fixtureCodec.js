// Decode shared rows and frame bodies once; fixture IDs retain shared arrays.
export function decodeFixtureDocument(data) {
  if (data?.schema === 1 && Array.isArray(data.fixtures)) return data;
  if (data?.schema !== 2 || !Array.isArray(data.rows) || !Array.isArray(data.frames) || !Array.isArray(data.fixtures)) throw new Error('Unsupported fixture document');
  const decodeRows = (ids) => ids.map((id) => {
    if (!Number.isInteger(id) || typeof data.rows[id] !== 'string') throw new Error('Invalid fixture row');
    return data.rows[id];
  });
  const frames = data.frames.map((frame) => ({ ...frame, lines: decodeRows(frame.lines), plain: decodeRows(frame.plain) }));
  const fixtures = data.fixtures.map(([id, index]) => {
    if (typeof id !== 'string' || !Number.isInteger(index) || !frames[index]) throw new Error('Invalid fixture reference');
    return { ...frames[index], id };
  });
  return { schema: data.schema, version: data.version, renderer: data.renderer, sourceFingerprint: data.sourceFingerprint, fixtures };
}
