export type UserType = 'client' | 'lawyer';

export type Role = 'user' | 'ai';

export interface Message {
  id: string;
  role: Role;
  text: string;
  thinking?: boolean;
}

export interface ChatResponse {
  reply: string;
  session_id: string;
  user_type: UserType;
  suggested_actions?: string[];
}

export interface ErrorResponse {
  error: string;
}

export interface BookingRequest {
  name: string;
  phone: string;
  email: string;
  date: string;   // YYYY-MM-DD
  time: string;   // HH:MM
  duration: number;
  topic: string;
}

export interface OfficeConfig {
  name: string;
  address: string;
  phone: string;
  email: string;
  hours: string;
}

export interface FormMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface FormExtractRequest {
  form_id: string;
  messages: FormMessage[];
}

export interface FormExtractResponse {
  form_id: string;
  form_name: string;
  values: Record<string, string>;
  missing: string[];
  summary: string;
}
