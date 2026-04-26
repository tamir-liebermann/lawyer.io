import { createTheme } from '@mui/material/styles';

// Legal-professional palette: off-white paper, deep navy, gold accent.
// Mirrors the palette the spec calls out in SYSTEM_PROMPT.md.
export const theme = createTheme({
  direction: 'rtl',
  palette: {
    mode: 'light',
    primary: {
      main: '#1A2A4A',
      dark: '#11203D',
      contrastText: '#F7F5F0',
    },
    secondary: {
      main: '#C9952A',
      contrastText: '#1A2A4A',
    },
    background: {
      default: '#F7F5F0',
      paper: '#FFFFFF',
    },
    text: {
      primary: '#2C2C2A',
      secondary: '#6B6B66',
    },
    divider: '#E8E4DC',
  },
  typography: {
    fontFamily:
      "'IBM Plex Sans', 'Noto Sans Hebrew', system-ui, -apple-system, Segoe UI, sans-serif",
    h1: { fontFamily: "'Playfair Display', serif", fontWeight: 700 },
    h2: { fontFamily: "'Playfair Display', serif", fontWeight: 700 },
    h3: { fontFamily: "'Playfair Display', serif", fontWeight: 600 },
    h6: { fontWeight: 600 },
    button: { textTransform: 'none', fontWeight: 500 },
  },
  shape: { borderRadius: 12 },
  components: {
    MuiButton: {
      styleOverrides: {
        root: { borderRadius: 12 },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: { backgroundImage: 'none' },
      },
    },
  },
});
