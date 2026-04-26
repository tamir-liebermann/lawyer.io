import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import { keyframes } from '@emotion/react';
import type { Message } from '../types';

// Three-dot loading animation for "thinking" bubbles.
const blink = keyframes`
  0%, 80%, 100% { opacity: 0.2; }
  40% { opacity: 1; }
`;

interface Props {
  message: Message;
}

export default function MessageBubble({ message }: Props) {
  const isUser = message.role === 'user';

  return (
    <Box
      sx={{
        display: 'flex',
        // In RTL layout, flex-start = right edge, flex-end = left edge.
        // Conventional chat UI: user on right, AI on left.
        justifyContent: isUser ? 'flex-start' : 'flex-end',
        width: '100%',
      }}
    >
      <Paper
        elevation={isUser ? 0 : 1}
        sx={{
          maxWidth: '78%',
          px: 2,
          py: 1.25,
          bgcolor: isUser ? 'primary.main' : 'background.paper',
          color: isUser ? 'primary.contrastText' : 'text.primary',
          border: isUser ? 0 : 1,
          borderColor: 'divider',
          borderRadius: 2.5,
          // Lower the inner corner closest to the bubble's "tail" side.
          borderEndEndRadius: isUser ? 6 : 20,
          borderEndStartRadius: isUser ? 20 : 6,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          boxShadow: isUser ? 'none' : '0 1px 2px rgba(0,0,0,0.04), 0 8px 24px rgba(26,42,74,0.06)',
        }}
      >
        <Typography variant="body2" component="div" sx={{ fontSize: 15, lineHeight: 1.6 }}>
          {message.text}
          {message.thinking && (
            <Box
              component="span"
              sx={{
                display: 'inline-flex',
                gap: 0.5,
                ml: 0.75,
                verticalAlign: 'middle',
              }}
              aria-label="טוען"
            >
              {[0, 1, 2].map((i) => (
                <Box
                  key={i}
                  component="span"
                  sx={{
                    width: 4,
                    height: 4,
                    borderRadius: '50%',
                    bgcolor: 'text.secondary',
                    animation: `${blink} 1.2s ease-in-out infinite both`,
                    animationDelay: `${i * 0.15}s`,
                  }}
                />
              ))}
            </Box>
          )}
        </Typography>
      </Paper>
    </Box>
  );
}
