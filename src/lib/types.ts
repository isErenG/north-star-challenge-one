export interface Place {
  id: string;
  displayName: { text: string };
  formattedAddress: string;
  businessStatus: string;
  internationalPhoneNumber: string;
  websiteUri: string;
  googleMapsUri: string;
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
  duplicateGroup?: string;
  mergedInto?: string;
  mergedFrom?: string[];
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
  error?: string;
}
