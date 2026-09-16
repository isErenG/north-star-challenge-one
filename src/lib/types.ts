export interface Place {
  id: string;
  displayName: { text: string };
  formattedAddress: string;
  businessStatus: string;
  internationalPhoneNumber: string;
  websiteUri: string;
  googleMapsUri: string;
  primaryType?: string;
  primaryTypeDisplayName?: { text: string };
  types?: string[];
}

/** Human-readable Google category for a candidate, or ''. */
export function placeIndustry(place?: Place): string {
  return place?.primaryTypeDisplayName?.text || place?.primaryType || '';
}
export interface Evidence {
  source: string;
  url?: string;
  note?: string;
}
export type SuggestionField =
  'name' | 'address' | 'phone' | 'email' | 'website' | 'activity' | 'status';
export interface Suggestion {
  field: SuggestionField;
  current: string;
  value: string;
  kind: 'confirmed' | 'different' | 'new';
  confidence: number;
  evidence: Evidence[];
  accepted?: boolean;
  /** Written into the record by the backend without review; `accepted` is also true. */
  auto?: boolean;
}
export type Stage =
  | 'checks'
  | 'google'
  | 'website'
  | 'databe'
  | 'goldenpages'
  | 'trendstop'
  | 'vkbo'
  | 'companyweb_demo'
  | 'judge'
  | 'extract'
  | 'reconcile';
export type StepStatus = 'running' | 'done' | 'skipped' | 'failed';
export interface Step {
  demo?: {
    url: string;
    capturedAt: string;
    enterprise: string;
    name: string;
    address: string;
    status: string;
  };
  stage: Stage;
  status: StepStatus;
  note: string;
  cost: number;
  findings: number;
  startedAt: string;
  endedAt?: string;
}
export interface Verification {
  verdict:
    | 'likely_active'
    | 'likely_ceased'
    | 'unclear'
    | 'skipped'
    | 'not_run'
    | 'running';
  reason: string;
  sources: string[];
  skipped?: string;
  cost: number;
  checkedAt: string;
  trace: Step[];
}
export interface Business {
  id: string;
  number: string;
  enterprise: string;
  kind: string;
  name: string;
  address: string;
  municipality: string;
  status: string;
  phone: string;
  email: string;
  website: string;
  notes: string;
  reviewed: boolean;
  issues: string[];
  geometry: unknown;
  source: Record<string, unknown>;
  google?: Place;
  googleError?: string;
  verification?: Verification;
  suggestions?: Suggestion[];
  duplicateGroup?: string;
  mergedInto?: string;
  mergedFrom?: string[];
}
export interface Enrichment {
  sources: string[];
  budgetEur: number;
  spentEur: number;
  verified: number;
  skipped: number;
  concurrency: number;
}
export interface Job {
  id: string;
  filename: string;
  created: string;
  state: 'processing' | 'done' | 'failed';
  phase: string;
  progress: number;
  total: number;
  records: Business[];
  email: string;
  notification: string;
  enrich: boolean;
  enrichment?: Enrichment;
  error?: string;
}
