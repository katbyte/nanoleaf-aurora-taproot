# Roadmap

What taproot does not do yet, roughly in the order it is likely to.

## Next

**The editor, further.** The page's scene editor makes and changes scenes, previews them on a controller's panels
or a wall built of triangles, shows them on real panels without saving, and keeps them in a library. Still to come
in it: painting a colour per panel by clicking (the API's `static` effects; `taproot scene paint` does it from the
command line), the motions other products have (the editor takes a controller's own list, so nothing stops them,
but the preview only knows the six built-in ones), and copying the built wall to a real layout.

**Renaming scenes from the page.** `taproot scene rename` does it from the command line; the page's scene menu
has copy, delete and edit, not rename.

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
