import type { Business, Job } from './types';
export function needsReview(row: Business) {
  return (
    !row.reviewed &&
    (row.issues.length > 0 || Boolean(row.google) || Boolean(row.googleError))
  );
}
export function csvCell(value: unknown): string {
  let text =
    value == null
      ? ''
      : typeof value === 'object'
        ? JSON.stringify(value)
        : String(value);
  // Prevent spreadsheet formula execution, including values preceded by whitespace.
  if (/^[\s]*[=+\-@]/.test(text)) text = "'" + text;
  return '"' + text.replaceAll('"', '""') + '"';
}
export function exportJob(job: Job, format: 'json' | 'csv' | 'geojson') {
  const base = {
    schemaVersion: 1,
    sourceFile: job.filename,
    exportedAt: new Date().toISOString(),
    records: job.records
  };
  if (format === 'json') return JSON.stringify(base, null, 2);
  if (format === 'geojson')
    return JSON.stringify(
      {
        type: 'FeatureCollection',
        sourceFile: job.filename,
        features: job.records.map(({ geometry, ...properties }) => ({
          type: 'Feature',
          geometry: geometry ?? null,
          properties
        }))
      },
      null,
      2
    );
  const sourceKeys = [
    ...new Set(job.records.flatMap((row) => Object.keys(row.source)))
  ];
  const fields = [
    'number',
    'enterprise',
    'kind',
    'name',
    'address',
    'municipality',
    'status',
    'phone',
    'email',
    'website',
    'notes',
    'reviewed',
    'issues',
    'geometry'
  ] as const;
  // Prefix review fields so no input column is overwritten, even with unusual source headers.
  let prefix = 'review_';
  const googleFields = [
    'name',
    'address',
    'status',
    'phone',
    'website',
    'maps_url',
    'error'
  ];
  const googleValues = (row: Business) => [
    row.google?.displayName.text,
    row.google?.formattedAddress,
    row.google?.businessStatus,
    row.google?.internationalPhoneNumber,
    row.google?.websiteUri,
    row.google?.googleMapsUri,
    row.googleError
  ];
  while (
    fields.some((key) => sourceKeys.includes(prefix + key)) || googleFields.some((key) =>
      sourceKeys.includes(prefix + 'google_candidate_' + key)
    )
  )
    prefix = '_' + prefix;
  const headers = [
    ...sourceKeys,
    ...fields.map((key) => prefix + key),
    ...googleFields.map((key) => prefix + 'google_candidate_' + key)
  ];
  return (
    '\ufeff' +
    [
      headers.map(csvCell).join(','),
      ...job.records.map((row) =>
        [
          ...sourceKeys.map((key) => row.source[key]),
          ...fields.map((key) =>
            key === 'issues' ? row.issues.join('; ') : row[key]
          ),
          ...googleValues(row)
        ]
          .map(csvCell)
          .join(',')
      )
    ].join('\r\n')
  );
}
export function download(content: string, filename: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
