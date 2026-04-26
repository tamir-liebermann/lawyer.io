import { useCallback, useEffect, useState } from 'react';
import type { Message, UserType } from '../types';
import { resetChat, sendChat, setServerMode } from '../api';

const GREETING: Message = {
  id: 'greeting',
  role: 'ai',
  text:
    'שלום! אני העוזר של המשרד. אפשר לשאול אותי מה להביא לפגישה, איך עובד תהליך רישום בטאבו, או כל שאלה נדל"נית אחרת. אם את/ה עורך/ת דין או איש/ת צוות, לחצו על "צוות המשרד" בתפריט.',
};

const DEFAULT_CHIPS_CLIENT = [
  'מה להביא לפגישה ראשונה',
  'כמה זמן לוקח רישום בטאבו?',
  'מה זה מס שבח?',
  'שלבי עסקת נדל"ן',
];
const DEFAULT_CHIPS_LAWYER = [
  'איסוף נתונים לטופס 7002',
  'איסוף נתונים לטופס 7000',
  'חיפוש עסקאות בתל אביב',
  'חוק המקרקעין סעיף 9',
];

// Initial mode from ?mode=lawyer URL param, default client.
function initialMode(): UserType {
  const params = new URLSearchParams(window.location.search);
  const m = params.get('mode');
  return m === 'lawyer' ? 'lawyer' : 'client';
}

function uid(): string {
  return Math.random().toString(36).slice(2) + Date.now().toString(36);
}

export function useChat() {
  const [mode, setModeState] = useState<UserType>(initialMode());
  const [messages, setMessages] = useState<Message[]>([GREETING]);
  const [suggestions, setSuggestions] = useState<string[]>(
    initialMode() === 'lawyer' ? DEFAULT_CHIPS_LAWYER : DEFAULT_CHIPS_CLIENT,
  );
  const [sending, setSending] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);

  // When mode changes, refresh default chips and tell the server.
  const setMode = useCallback((next: UserType) => {
    setModeState(next);
    setSuggestions(next === 'lawyer' ? DEFAULT_CHIPS_LAWYER : DEFAULT_CHIPS_CLIENT);
    void setServerMode(next);
  }, []);

  // On first mount, make sure the URL param's mode is also persisted server-side.
  useEffect(() => {
    void setServerMode(initialMode());
  }, []);

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || sending) return;

      setLastError(null);
      const userMsg: Message = { id: uid(), role: 'user', text: trimmed };
      const thinking: Message = { id: uid(), role: 'ai', text: 'חושב', thinking: true };

      setMessages((prev) => [...prev, userMsg, thinking]);
      setSending(true);

      try {
        const data = await sendChat(trimmed, mode);
        setMessages((prev) => {
          const withoutThinking = prev.filter((m) => m.id !== thinking.id);
          return [...withoutThinking, { id: uid(), role: 'ai', text: data.reply }];
        });
        if (data.user_type && data.user_type !== mode) {
          setModeState(data.user_type);
        }
        if (data.suggested_actions && data.suggested_actions.length) {
          setSuggestions(data.suggested_actions);
        }
      } catch (e) {
        const err = e instanceof Error ? e.message : String(e);
        setLastError(err);
        setMessages((prev) => {
          const withoutThinking = prev.filter((m) => m.id !== thinking.id);
          return [
            ...withoutThinking,
            { id: uid(), role: 'ai', text: `שגיאה: ${err}` },
          ];
        });
      } finally {
        setSending(false);
      }
    },
    [mode, sending],
  );

  const reset = useCallback(async () => {
    await resetChat();
    setMessages([{ ...GREETING, id: uid() }]);
    setSuggestions(mode === 'lawyer' ? DEFAULT_CHIPS_LAWYER : DEFAULT_CHIPS_CLIENT);
    setLastError(null);
  }, [mode]);

  return {
    mode,
    setMode,
    messages,
    suggestions,
    sending,
    lastError,
    send,
    reset,
  };
}

export type UseChat = ReturnType<typeof useChat>;
