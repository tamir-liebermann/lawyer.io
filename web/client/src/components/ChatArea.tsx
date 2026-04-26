import { useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Stack from '@mui/material/Stack';
import MessageBubble from './MessageBubble';
import SuggestedChips from './SuggestedChips';
import Composer from './Composer';
import type { Message } from '../types';

interface Props {
  messages: Message[];
  suggestions: string[];
  sending: boolean;
  onSend: (text: string) => void;
  officeName: string;
}

export default function ChatArea({ messages, suggestions, sending, onSend, officeName }: Props) {
  const [draft, setDraft] = useState('');
  const scrollRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom on new messages.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

  const submit = () => {
    const trimmed = draft.trim();
    if (!trimmed) return;
    onSend(trimmed);
    setDraft('');
  };

  const pickChip = (text: string) => {
    setDraft(text);
  };

  return (
    <Box
      component="main"
      sx={{
        display: 'grid',
        gridTemplateRows: 'auto 1fr auto auto',
        maxHeight: '100vh',
        minHeight: 0,
      }}
    >
      {/* Header */}
      <Box
        sx={{
          px: 3.5,
          pt: 2.5,
          pb: 1.25,
          borderBottom: 1,
          borderColor: 'divider',
          bgcolor: 'background.default',
        }}
      >
        <Typography
          variant="h5"
          component="h1"
          sx={{ fontFamily: "'Playfair Display', serif", color: 'primary.main', fontWeight: 700 }}
        >
          {officeName} — עוזר AI משפטי נדל"ן
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          שירות מידע בלבד — אינו מהווה ייעוץ משפטי מחייב.
        </Typography>
      </Box>

      {/* Messages */}
      <Box
        ref={scrollRef}
        sx={{
          overflowY: 'auto',
          px: 3.5,
          py: 3,
          minHeight: 0,
        }}
        aria-live="polite"
      >
        <Stack spacing={1.75}>
          {messages.map((m) => (
            <MessageBubble key={m.id} message={m} />
          ))}
        </Stack>
      </Box>

      <SuggestedChips items={suggestions} onPick={pickChip} />

      <Composer
        value={draft}
        onChange={setDraft}
        onSubmit={submit}
        sending={sending}
      />
    </Box>
  );
}
