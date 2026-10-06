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

**Painting panels.** A static scene with a colour chosen for each panel (the API's `static` effects and their
`animData`), by clicking panels on the page.

**Deleting and renaming scenes.** `sdk/aurora` has both. Neither has a command or a button, because neither can be
undone short of a restore; they want a confirmation, and a backup taken first, like every other write.

**Restoring from the page.** The page takes backups and lists none. Restoring is `taproot restore`.

## Later

- colour and white modes on the page: one colour by hue and saturation, a white by temperature
- schedules, which firmware 5 reports and the API does not document
- the Rhythm module's settings: microphone or aux
- other Nanoleaf products. Nothing blocks them where it costs nothing, since the API is shared: the page draws
  squares and hexagons, and the SDK has the shapes. None has been tried

## Not planned

- anything through Nanoleaf's cloud, or the Discover catalogue of scenes
- streaming colours to the panels live (external control), and screen mirroring
