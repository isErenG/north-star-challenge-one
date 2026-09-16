import type { Business } from './types';

/** Longitude/latitude pair in GeoJSON order. */
export type LngLat = [number, number];

function isLngLat(value: unknown): value is LngLat {
  return (
    Array.isArray(value) &&
    value.length >= 2 &&
    typeof value[0] === 'number' &&
    typeof value[1] === 'number' &&
    Number.isFinite(value[0]) &&
    Number.isFinite(value[1]) &&
    value[0] >= -180 &&
    value[0] <= 180 &&
    value[1] >= -90 &&
    value[1] <= 90
  );
}

/**
 * Returns a representative point for a record's geometry, or null when the
 * record has no usable coordinates. Points are used as-is; other geometry
 * types fall back to their first position so no record is silently dropped.
 * Nothing is ever invented for records without geometry.
 */
export function pointOf(geometry: unknown): LngLat | null {
  if (!geometry || typeof geometry !== 'object') return null;
  const coords = (geometry as { coordinates?: unknown }).coordinates;
  let cursor: unknown = coords;
  for (let depth = 0; depth < 4; depth++) {
    if (isLngLat(cursor)) return [cursor[0], cursor[1]];
    if (!Array.isArray(cursor) || cursor.length === 0) return null;
    cursor = cursor[0];
  }
  return null;
}

export function located(rows: Business[]) {
  return rows.filter((row) => pointOf(row.geometry) !== null);
}
