-- Channel reads and appearance updates use this optional display-count override.
ALTER TABLE public.channels ADD COLUMN IF NOT EXISTS fake_participants integer NOT NULL DEFAULT 0;
