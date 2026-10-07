# Roadmap

What taproot does not do yet, roughly in the order it is likely to.

## Next

**A scene editor on the page.** Make a scene, or change one: pick the motion, build the palette, set the speed and
the direction, watch it on the drawn panels, try it on the real ones without saving (the API's `display`), then
save it. This is the part of the app taproot does not replace yet. What is there to build on:

- `Client.Plugins` lists the motions a controller has and, for each, the options it takes with their types, limits
  and defaults, which is everything a form needs
- the page already draws the six built-in motions from a palette and their options (`sampler` in `assets/page.js`)
- `Client.DisplayEffect` shows a scene on the panels without storing it, and `Client.AddEffect` stores it
- a scene would still be sent as a controller's own document: start from one the controller holds and change
  fields, rather than building one from nothing, so whatever else the firmware wants stays in it

**Painting panels on the page.** `taproot scene paint` does it from the command line (the API's `static` effects and
their `animData`); the page has no way to click a panel and pick a colour yet.

**Renaming and deleting scenes from the page.** `taproot scene rename` and `scene delete` do both from the command
line, with a backup first. The page has no button for either.

**Restoring from the page.** The page takes backups and lists none. Restoring is `taproot restore`.

## Later

- colour and white modes on the page: one colour by hue and saturation, a white by temperature (`taproot set` has them)
- one colour by name or hex code, `taproot set office colour ff8800`, in place of hue and saturation apart
- schedules, which firmware 5 reports and the API does not document
- the Rhythm module's settings: microphone or aux
- other Nanoleaf products. Nothing blocks them where it costs nothing, since the API is shared: the page draws
  squares and hexagons, and the SDK has the shapes. None has been tried

## Not planned

- anything through Nanoleaf's cloud, or the Discover catalogue of scenes
- streaming colours to the panels live (external control), and screen mirroring
