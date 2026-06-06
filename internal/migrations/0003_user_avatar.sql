-- Store the player's avatar URL from the auth provider (Telegram photo_url /
-- Discord avatar), shown in the web app profile.
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;
