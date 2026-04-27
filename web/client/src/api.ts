import type { BookingRequest, ChatResponse, ErrorResponse, FormExtractRequest, FormExtractResponse, OfficeConfig, UserType } from './types';

// All endpoints are same-origin; credentials must include so the
// gorilla/sessions cookie (lawyer_session) roundtrips.
const FETCH_OPTS: RequestInit = { credentials: 'same-origin' };

export async function sendChat(message: string, mode: UserType): Promise<ChatResponse> {
  const res = await fetch('/api/chat', {
    ...FETCH_OPTS,
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ message, mode }),
  });
  const text = await res.text();
  let data: ChatResponse | ErrorResponse;
  try {
    data = JSON.parse(text);
  } catch {
    throw new Error(text || 'תגובה לא תקינה מהשרת');
  }
  if (!res.ok) {
    const err = data as ErrorResponse;
    throw new Error(err.error || `שגיאה: ${res.status}`);
  }
  return data as ChatResponse;
}

export async function setServerMode(mode: UserType): Promise<void> {
  // Non-fatal if it fails; /api/chat also persists the mode.
  try {
    await fetch('/api/mode', {
      ...FETCH_OPTS,
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ mode }),
    });
  } catch {
    /* ignore */
  }
}

export async function resetChat(): Promise<void> {
  try {
    await fetch('/api/reset', { ...FETCH_OPTS, method: 'POST' });
  } catch {
    /* ignore */
  }
}

export async function bookMeeting(req: BookingRequest): Promise<void> {
  const res = await fetch('/api/book', {
    ...FETCH_OPTS,
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `שגיאה: ${res.status}`);
  }
}

export async function extractFormData(req: FormExtractRequest): Promise<FormExtractResponse> {
  const res = await fetch('/api/forms/extract', {
    ...FETCH_OPTS,
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(req),
  });
  const text = await res.text();
  if (!res.ok) throw new Error(text || `שגיאה: ${res.status}`);
  return JSON.parse(text) as FormExtractResponse;
}

export async function downloadFormPDF(
  formId: string,
  values: Record<string, string>,
): Promise<Blob> {
  const res = await fetch('/api/forms/pdf', {
    ...FETCH_OPTS,
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ form_id: formId, values }),
  });
  if (!res.ok) throw new Error((await res.text()) || `שגיאה: ${res.status}`);
  return res.blob();
}

export async function fetchOffice(): Promise<OfficeConfig> {
  const res = await fetch('/api/office', FETCH_OPTS);
  if (!res.ok) throw new Error(`office fetch: ${res.status}`);
  return res.json() as Promise<OfficeConfig>;
}
