import type { Business } from './types';

export function normalizeSearch(value: string): string {
  return value
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();
}

function values(value: unknown): string[] {
  if (value == null) return [];
  if (Array.isArray(value)) return value.flatMap(values);
  if (typeof value === 'object') return Object.values(value).flatMap(values);
  return [String(value)];
}

// Recognise source headers, including nested JSON and common NL/FR exports.
function sourceLabels(
  source: Record<string, unknown>,
  pattern: RegExp
): string[] {
  const result: string[] = [];
  function visit(value: unknown) {
    if (!value || typeof value !== 'object') return;
    for (const [key, child] of Object.entries(value)) {
      const header = normalizeSearch(key).replaceAll(' ', '');
      // Classification versions and activity counts are metadata, not labels.
      if (/^(nace.*(?:versie|version)|aantalhoofdact)/.test(header)) continue;
      if (pattern.test(header)) {
        result.push(
          ...values(child)
            .flatMap((text) => text.split(/[;|\n]/))
            .map((text) => text.trim())
            .filter(Boolean)
        );
      } else visit(child);
    }
  }
  visit(source);
  return [...new Set(result)];
}

export function industries(source: Record<string, unknown>): string[] {
  return sourceLabels(
    source,
    /industry|industries|industrie|sector|nace|omschrijvinghoofdact|activity|activities|activiteit|activiteiten|activite|branche/
  );
}

export function categories(source: Record<string, unknown>): string[] {
  return sourceLabels(
    source,
    /category|categories|categorie|kategorie|^nacehoofdact|^omschrijvinghoofdact/
  );
}

export function indexBusiness(row: Business) {
  return {
    row,
    industries: industries(row.source),
    categories: categories(row.source),
    text: normalizeSearch(
      [
        row.name,
        row.number,
        row.enterprise,
        row.address,
        row.municipality,
        row.status,
        row.phone,
        row.email,
        row.website,
        row.notes,
        ...sourceLabels(row.source, /^nace(?!.*(?:versie|version))/).map(
          (code) => code.replace(/[.\s]/g, '')
        ),
        ...values(row.source)
      ].join(' ')
    )
  };
}

export function matchesSearch(text: string, query: string): boolean {
  return normalizeSearch(query)
    .split(' ')
    .filter(Boolean)
    .every((term) => text.includes(term));
}
