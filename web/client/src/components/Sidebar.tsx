import { useState } from 'react';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Button from '@mui/material/Button';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Divider from '@mui/material/Divider';
import PersonOutlineIcon from '@mui/icons-material/PersonOutline';
import GavelIcon from '@mui/icons-material/Gavel';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import EventAvailableIcon from '@mui/icons-material/EventAvailable';
import BookingModal from './BookingModal';
import type { OfficeConfig, UserType } from '../types';

const FORM_EXTRACT_ITEMS = [
  { id: '7002', label: 'טופס 7002' },
  { id: '7000', label: 'טופס 7000' },
  { id: 'tabu_registration', label: 'רישום טאבו' },
];

const CLIENT_TOOLS = [
  'מה להביא לפגישה ראשונה',
  'שלבי עסקת נדל"ן',
  'מס שבח — הסבר פשוט',
];
const LAWYER_TOOLS = [
  'איסוף נתונים לטופס 7002',
  'איסוף נתונים לטופס 7000',
  'חיפוש עסקאות בתל אביב',
  'חוק המקרקעין — סעיפים מרכזיים',
];

interface Props {
  mode: UserType;
  onModeChange: (m: UserType) => void;
  onQuickTool: (text: string) => void;
  onReset: () => void;
  officeConfig: OfficeConfig;
  onExtractForm: (formId: string) => void;
}

export default function Sidebar({ mode, onModeChange, onQuickTool, onReset, officeConfig, onExtractForm }: Props) {
  const tools = mode === 'lawyer' ? LAWYER_TOOLS : CLIENT_TOOLS;
  const [bookingOpen, setBookingOpen] = useState(false);

  return (
    <Box
      component="aside"
      sx={{
        bgcolor: 'background.paper',
        borderInlineEnd: 1,
        borderColor: 'divider',
        p: 3,
        display: 'flex',
        flexDirection: 'column',
        gap: 3,
        overflowY: 'auto',
      }}
    >
      {/* Brand */}
      <Stack direction="row" spacing={1.5} alignItems="center">
        <Box
          sx={{
            width: 44,
            height: 44,
            borderRadius: 2,
            bgcolor: 'primary.main',
            color: 'secondary.main',
            display: 'grid',
            placeItems: 'center',
            fontFamily: "'Playfair Display', serif",
            fontWeight: 700,
            fontSize: 22,
          }}
        >
          L
        </Box>
        <Box>
          <Typography
            variant="h6"
            sx={{ fontFamily: "'Playfair Display', serif", color: 'primary.main', lineHeight: 1.1 }}
          >
            Lawyer.io
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ letterSpacing: '0.04em' }}>
            נדל"ן · עו"ד
          </Typography>
        </Box>
      </Stack>

      {/* Office contact details — only rendered when configured */}
      {(officeConfig.address || officeConfig.phone || officeConfig.hours) && (
        <Box sx={{ mt: -1.5 }}>
          {officeConfig.address && (
            <Typography variant="caption" color="text.secondary" display="block">
              {officeConfig.address}
            </Typography>
          )}
          {officeConfig.phone && (
            <Typography variant="caption" color="text.secondary" display="block">
              {officeConfig.phone}
            </Typography>
          )}
          {officeConfig.hours && (
            <Typography variant="caption" color="text.secondary" display="block">
              {officeConfig.hours}
            </Typography>
          )}
        </Box>
      )}

      {/* Mode toggle */}
      <Box>
        <Typography
          variant="overline"
          color="text.secondary"
          display="block"
          sx={{ mb: 1, letterSpacing: '0.08em', fontWeight: 600 }}
        >
          סוג משתמש
        </Typography>
        <ToggleButtonGroup
          value={mode}
          exclusive
          onChange={(_, next: UserType | null) => {
            if (next) onModeChange(next);
          }}
          fullWidth
          size="small"
          color="primary"
          aria-label="סוג משתמש"
        >
          <ToggleButton value="client" aria-label="לקוח">
            <PersonOutlineIcon fontSize="small" sx={{ ml: 0.75 }} />
            לקוח
          </ToggleButton>
          <ToggleButton value="lawyer" aria-label="צוות המשרד">
            <GavelIcon fontSize="small" sx={{ ml: 0.75 }} />
            צוות המשרד
          </ToggleButton>
        </ToggleButtonGroup>
      </Box>

      <Divider />

      {/* Quick tools */}
      <Box>
        <Typography
          variant="overline"
          color="text.secondary"
          display="block"
          sx={{ mb: 1, letterSpacing: '0.08em', fontWeight: 600 }}
        >
          כלים מהירים
        </Typography>
        <List dense disablePadding>
          {tools.map((t) => (
            <ListItem key={t} disablePadding sx={{ mb: 0.5 }}>
              <ListItemButton
                onClick={() => onQuickTool(t)}
                sx={{
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 2,
                  bgcolor: 'background.default',
                  '&:hover': { borderColor: 'secondary.main', bgcolor: '#FBF9F3' },
                }}
              >
                <ListItemText
                  primary={t}
                  primaryTypographyProps={{ fontSize: 14 }}
                />
              </ListItemButton>
            </ListItem>
          ))}
        </List>
      </Box>

      {/* Form data extraction — lawyers only */}
      {mode === 'lawyer' && (
        <>
          <Divider />
          <Box>
            <Typography
              variant="overline"
              color="text.secondary"
              display="block"
              sx={{ mb: 1, letterSpacing: '0.08em', fontWeight: 600 }}
            >
              חילוץ נתוני טופס
            </Typography>
            <Stack direction="column" spacing={1}>
              {FORM_EXTRACT_ITEMS.map(({ id, label }) => (
                <Button
                  key={id}
                  variant="outlined"
                  size="small"
                  fullWidth
                  onClick={() => onExtractForm(id)}
                  sx={{
                    borderColor: 'secondary.main',
                    color: 'primary.main',
                    fontWeight: 600,
                    fontSize: 13,
                    '&:hover': { bgcolor: '#FBF9F3' },
                  }}
                >
                  {label}
                </Button>
              ))}
            </Stack>
          </Box>
        </>
      )}

      {/* Book a meeting — clients only */}
      {mode === 'client' && (
        <>
          <Divider />
          <Button
            variant="contained"
            fullWidth
            startIcon={<EventAvailableIcon />}
            onClick={() => setBookingOpen(true)}
            sx={{ fontWeight: 600 }}
          >
            קביעת פגישה
          </Button>
        </>
      )}

      {/* Reset */}
      <Box sx={{ mt: 'auto' }}>
        <Button
          variant="outlined"
          color="inherit"
          fullWidth
          startIcon={<RestartAltIcon />}
          onClick={onReset}
          sx={{ borderColor: 'divider', color: 'text.secondary' }}
        >
          התחל שיחה חדשה
        </Button>
      </Box>

      <BookingModal open={bookingOpen} onClose={() => setBookingOpen(false)} />
    </Box>
  );
}
