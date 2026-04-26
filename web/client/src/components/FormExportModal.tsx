import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import DownloadIcon from '@mui/icons-material/Download';
import type { FormExtractResponse } from '../types';

interface Props {
  open: boolean;
  result: FormExtractResponse | null;
  onClose: () => void;
}

export default function FormExportModal({ open, result, onClose }: Props) {
  if (!result) return null;

  const handleDownload = () => {
    const json = JSON.stringify(result.values, null, 2);
    const blob = new Blob([json], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${result.form_id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
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
        {result.form_name}
      </DialogTitle>

      <DialogContent sx={{ pt: 1.5 }}>
        <Typography variant="body2" color="text.primary" sx={{ whiteSpace: 'pre-line', mb: 2 }}>
          {result.summary}
        </Typography>
        {(result.missing ?? []).length > 0 && (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, alignItems: 'center' }}>
            <Typography variant="caption" color="text.secondary">
              שדות חסרים:
            </Typography>
            {result.missing.map((f) => (
              <Chip key={f} label={f} size="small" color="warning" variant="outlined" />
            ))}
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2.5, gap: 1 }}>
        <Button onClick={onClose} color="inherit">
          סגור
        </Button>
        <Button
          onClick={handleDownload}
          variant="contained"
          startIcon={<DownloadIcon />}
          sx={{ minWidth: 140 }}
        >
          הורד JSON
        </Button>
      </DialogActions>
    </Dialog>
  );
}
