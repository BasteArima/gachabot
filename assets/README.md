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
