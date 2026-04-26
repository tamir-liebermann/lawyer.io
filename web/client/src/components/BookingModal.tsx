import { useState } from 'react';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import { bookMeeting } from '../api';
import type { BookingRequest } from '../types';

interface Props {
  open: boolean;
  onClose: () => void;
}

const EMPTY: BookingRequest = {
  name: '', phone: '', email: '',
  date: '', time: '', duration: 60, topic: '',
};

// Minimum date = today in YYYY-MM-DD format (local time).
function todayISO(): string {
  const d = new Date();
  return d.toISOString().split('T')[0];
}

export default function BookingModal({ open, onClose }: Props) {
  const [form, setForm] = useState<BookingRequest>(EMPTY);
  const [sending, setSending] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState('');

  const set = (field: keyof BookingRequest) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((prev) => ({ ...prev, [field]: e.target.value }));

  const valid = form.name.trim() && form.phone.trim() && form.date && form.time;

  const handleSubmit = async () => {
    if (!valid || sending) return;
    setSending(true);
    setError('');
    try {
      await bookMeeting({ ...form, duration: 60 });
      setDone(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'אירעה שגיאה, נסה שוב');
    } finally {
      setSending(false);
    }
  };

  const handleClose = () => {
    if (sending) return;
    onClose();
    // Reset after dialog close animation finishes.
    setTimeout(() => { setForm(EMPTY); setDone(false); setError(''); }, 300);
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      fullWidth
      maxWidth="sm"
      PaperProps={{ sx: { borderRadius: 3 } }}
    >
      <DialogTitle
        sx={{
          fontFamily: "'Playfair Display', serif",
          color: 'primary.main',
          fontWeight: 700,
          pb: 0.5,
        }}
      >
        קביעת פגישה
      </DialogTitle>

      <DialogContent sx={{ pt: 1.5 }}>
        {done ? (
          <Box sx={{ textAlign: 'center', py: 4 }}>
            <CheckCircleOutlineIcon sx={{ fontSize: 56, color: 'secondary.main', mb: 1.5 }} />
            <Typography variant="h6" color="primary.main" fontWeight={600}>
              הפגישה נקבעה בהצלחה
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              המשרד יצור איתך קשר לאישור הסופי.
            </Typography>
          </Box>
        ) : (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 0.5 }}>
            <TextField
              label="שם מלא"
              value={form.name}
              onChange={set('name')}
              required
              fullWidth
              size="small"
            />
            <TextField
              label="טלפון"
              value={form.phone}
              onChange={set('phone')}
              required
              fullWidth
              size="small"
              inputProps={{ dir: 'ltr' }}
            />
            <TextField
              label="אימייל (רשות)"
              value={form.email}
              onChange={set('email')}
              fullWidth
              size="small"
              inputProps={{ dir: 'ltr' }}
            />
            <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2 }}>
              <TextField
                label="תאריך"
                type="date"
                value={form.date}
                onChange={set('date')}
                required
                size="small"
                InputLabelProps={{ shrink: true }}
                inputProps={{ min: todayISO() }}
              />
              <TextField
                label="שעה"
                type="time"
                value={form.time}
                onChange={set('time')}
                required
                size="small"
                InputLabelProps={{ shrink: true }}
                inputProps={{ step: 900 }} // 15-min steps
              />
            </Box>
            <TextField
              label="נושא הפגישה (רשות)"
              value={form.topic}
              onChange={set('topic')}
              fullWidth
              size="small"
              multiline
              rows={2}
              placeholder="למשל: רכישת דירה, עסקת מכירה, שכירות..."
            />
            {error && (
              <Typography variant="caption" color="error">
                {error}
              </Typography>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2.5, gap: 1 }}>
        {done ? (
          <Button onClick={handleClose} variant="contained" fullWidth>
            סגור
          </Button>
        ) : (
          <>
            <Button onClick={handleClose} color="inherit" disabled={sending}>
              ביטול
            </Button>
            <Button
              onClick={handleSubmit}
              variant="contained"
              disabled={!valid || sending}
              startIcon={sending ? <CircularProgress size={16} color="inherit" /> : null}
              sx={{ minWidth: 120 }}
            >
              {sending ? 'שולח...' : 'קבע פגישה'}
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}
