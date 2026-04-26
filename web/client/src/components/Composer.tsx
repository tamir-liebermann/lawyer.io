import React, { useRef } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import SendIcon from '@mui/icons-material/Send';
import CircularProgress from '@mui/material/CircularProgress';

interface Props {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  sending: boolean;
}

export default function Composer({ value, onChange, onSubmit, sending }: Props) {
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Enter sends, Shift+Enter newline.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      onSubmit();
    }
  };

  return (
    <Box
      component="form"
      onSubmit={(e: React.FormEvent) => {
        e.preventDefault();
        onSubmit();
      }}
      sx={{
        display: 'grid',
        gridTemplateColumns: '1fr auto',
        gap: 1.25,
        px: 3.5,
        py: 2.5,
        borderTop: 1,
        borderColor: 'divider',
        bgcolor: 'background.default',
      }}
    >
      <TextField
        inputRef={inputRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="כתוב/י שאלה..."
        multiline
        maxRows={6}
        fullWidth
        size="small"
        variant="outlined"
        aria-label="שדה הודעה"
        sx={{
          bgcolor: 'background.paper',
          '& .MuiOutlinedInput-root': { borderRadius: 3 },
        }}
      />
      <Button
        type="submit"
        variant="contained"
        disabled={sending || !value.trim()}
        startIcon={
          sending ? (
            <CircularProgress size={16} color="inherit" />
          ) : (
            <SendIcon sx={{ transform: 'scaleX(-1)' }} />
          )
        }
        sx={{ px: 3 }}
      >
        שלח
      </Button>
    </Box>
  );
}
