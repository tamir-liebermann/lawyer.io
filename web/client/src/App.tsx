import { useEffect, useState } from 'react';
import Box from '@mui/material/Box';
import useMediaQuery from '@mui/material/useMediaQuery';
import { useTheme } from '@mui/material/styles';

import Sidebar from './components/Sidebar';
import ChatArea from './components/ChatArea';
import FormExportModal from './components/FormExportModal';
import { useChat } from './hooks/useChat';
import { extractFormData, fetchOffice } from './api';
import type { FormExtractResponse, FormMessage, OfficeConfig } from './types';

const DEFAULT_OFFICE: OfficeConfig = {
  name: 'Lawyer.io',
  address: '',
  phone: '',
  email: '',
  hours: '',
};

export default function App() {
  const chat = useChat();
  const theme = useTheme();
  const isDesktop = useMediaQuery(theme.breakpoints.up('md'));
  const [office, setOffice] = useState<OfficeConfig>(DEFAULT_OFFICE);
  const [formResult, setFormResult] = useState<FormExtractResponse | null>(null);
  const [formModalOpen, setFormModalOpen] = useState(false);

  useEffect(() => {
    fetchOffice()
      .then(setOffice)
      .catch(() => { /* keep defaults silently */ });
  }, []);

  const handleExtractForm = async (formId: string) => {
    const msgs: FormMessage[] = chat.messages
      .filter((m) => !m.thinking)
      .map((m) => ({
        role: m.role === 'user' ? 'user' : 'assistant' as const,
        content: m.text,
      }));
    try {
      const result = await extractFormData({ form_id: formId, messages: msgs });
      setFormResult(result);
      setFormModalOpen(true);
    } catch {
      /* silently ignore */
    }
  };

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: isDesktop ? '280px 1fr' : '1fr',
        height: '100vh',
        bgcolor: 'background.default',
      }}
    >
      <Sidebar
        mode={chat.mode}
        onModeChange={chat.setMode}
        onQuickTool={(t) => chat.send(t)}
        onReset={() => void chat.reset()}
        officeConfig={office}
        onExtractForm={handleExtractForm}
      />
      <ChatArea
        messages={chat.messages}
        suggestions={chat.suggestions}
        sending={chat.sending}
        onSend={chat.send}
        officeName={office.name}
      />
      <FormExportModal
        open={formModalOpen}
        result={formResult}
        onClose={() => setFormModalOpen(false)}
      />
    </Box>
  );
}
