# assets/

Runtime asset files baked into the Docker image (copied to `/root/assets`).

## artguess_overlay.png

Optional overlay for the **Art Guess** chat board image. The bot renders today's
card at maximum blur and, if this file exists, composites it on top, then attaches
the result to the scoreboard / morning-ping message.

- **Size:** make it the card size (**1000 × 1316**, same standard as the card art).
  Other sizes are auto-resized to fit, but matching avoids distortion.
- **Format:** PNG with transparency — only the non-transparent pixels (your logo /
  banner / frame) show; the rest reveals the blurred art underneath.
- **Where to put it:**
  - Commit it here as `assets/artguess_overlay.png` → it's baked into the image on
    the next build (CI on push to `main`). Simplest.
  - Or, to swap without rebuilding: bind-mount your PNG into the container and set
    `ARTGUESS_OVERLAY_PATH` to its path (e.g. `/root/assets/artguess_overlay.png`).
- **If the file is absent:** the board simply shows the blurred art with no overlay.

## frames/

Optional card frames, one PNG per rarity folder name used by `gen.py`
(`frames/Rare.png`, `frames/Mythical.png`, …). The generator composites these
over the prepared art to produce `cards_framed/`; the bot itself never draws a
frame, it only swaps `/cards/` for `/cards_framed/` in the url.

The admin panel serves them back so a raw file can be tried on **before** the
generator has run: drop art into the card editor and the "с рамкой" preview
crops it to 1000 × 1316 and lays the chosen frame on top, exactly as `gen.py`
would.

- **Size:** 1000 × 1316, the card standard.
- **Format:** PNG with transparency.
- **Names:** must match the `raw_art` folder names (English, case-sensitive) —
  the rarity names in the database are localised and do not match.
- **Where to put it:** drop the folder here (it is gitignored, like the Art Guess
  overlay) so it is baked into the image, or bind-mount it and point
  `CARD_FRAMES_DIR` at it (e.g. `/root/assets/frames`).
- **If the folder is absent:** the panel simply offers no frames to try on.
