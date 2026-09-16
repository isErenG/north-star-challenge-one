import { placeIndustry } from './types';
import type { Business, Job, Suggestion, SuggestionField } from './types';
export const SUGGESTION_FIELDS: SuggestionField[] = [
  'name',
  'address',
  'phone',
  'email',
  'website',
  'activity',
  'status'
];
export function pendingSuggestions(row: Business): Suggestion[] {
  return (row.suggestions ?? []).filter(
    (s) => !s.accepted && s.kind !== 'confirmed'
  );
}
export function autoVerified(row: Business): Suggestion[] {
  return (row.suggestions ?? []).filter((s) => s.auto && s.accepted);
}
export function needsReview(row: Business) {
  if (row.reviewed) return false;
  const verdict = row.verification?.verdict;
  return (
    row.issues.length > 0 ||
    Boolean(row.googleError) ||
    verdict === 'likely_ceased' ||
    verdict === 'unclear' ||
    pendingSuggestions(row).length > 0
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
    'industry',
    'maps_url',
    'error'
  ];
  const googleValues = (row: Business) => [
    row.google?.displayName.text,
    row.google?.formattedAddress,
    row.google?.businessStatus,
    row.google?.internationalPhoneNumber,
    row.google?.websiteUri,
    placeIndustry(row.google),
    row.google?.googleMapsUri,
    row.googleError
  ];
  const verdictFields = ['verdict', 'verdict_reason', 'auto_verified'];
  const suggestedValue = (row: Business, field: SuggestionField) =>
    row.suggestions?.find((s) => s.field === field)?.value ?? '';
  const verificationValues = (row: Business) => [
    row.verification?.verdict,
    row.verification?.reason,
    autoVerified(row)
      .map((s) => s.field)
      .join('; '),
    ...SUGGESTION_FIELDS.map((field) => suggestedValue(row, field))
  ];
  while (
    fields.some((key) => sourceKeys.includes(prefix + key)) ||
    googleFields.some((key) =>
      sourceKeys.includes(prefix + 'google_candidate_' + key)
    ) ||
    verdictFields.some((key) => sourceKeys.includes(prefix + key)) ||
    SUGGESTION_FIELDS.some((key) =>
      sourceKeys.includes(prefix + 'suggested_' + key)
    )
  )
    prefix = '_' + prefix;
  const headers = [
    ...sourceKeys,
    ...fields.map((key) => prefix + key),
    ...googleFields.map((key) => prefix + 'google_candidate_' + key),
    ...verdictFields.map((key) => prefix + key),
    ...SUGGESTION_FIELDS.map((key) => prefix + 'suggested_' + key)
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
          ...googleValues(row),
          ...verificationValues(row)
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
