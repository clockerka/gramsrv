-- These columns were imported below the already published widget migration head.
-- Reapply their idempotent definitions so existing installations receive them too.
ALTER TABLE users ADD COLUMN IF NOT EXISTS custom_label TEXT NOT NULL DEFAULT '';
ALTER TABLE public.channels
  ADD COLUMN IF NOT EXISTS emoji_status_collectible_id bigint REFERENCES public.unique_star_gifts(id),
  ADD COLUMN IF NOT EXISTS emoji_status_collectible jsonb DEFAULT '{}'::jsonb NOT NULL;
