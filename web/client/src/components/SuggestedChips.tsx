import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';

interface Props {
  items: string[];
  onPick: (text: string) => void;
}

export default function SuggestedChips({ items, onPick }: Props) {
  if (!items || items.length === 0) return null;
  return (
    <Box
      sx={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 1,
        px: 3.5,
        pb: 1,
      }}
    >
      {items.map((it) => (
        <Chip
          key={it}
          label={it}
          onClick={() => onPick(it)}
          variant="outlined"
          sx={{
            bgcolor: 'background.paper',
            borderColor: 'divider',
            '&:hover': { borderColor: 'secondary.main' },
          }}
        />
      ))}
    </Box>
  );
}
