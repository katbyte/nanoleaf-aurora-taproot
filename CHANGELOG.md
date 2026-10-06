## v0.1.0 (unreleased)

Initial release.

- `taproot find`, `connect`, `list`, `info` and `forget`: find Nanoleaf Light Panels controllers on the network, get a token from each with one press of the power button, and keep the tokens in a file only you can read
- `taproot scene list`, `dump`, `push`, `copy` and `select`: read scenes, copy one from the controller that has it to the ones that do not, and start it
- `taproot backup` and `restore`: every scene of a controller to a directory of files, and back onto any controller
- nothing on a controller is replaced without `--force`, a controller is backed up before anything is written to it, every scene is read back after it is written, and `--dry-run` prints the exact request instead of sending it
- `taproot serve`: a page with every controller's panels drawn where they sit and moving as their scene does, with power, brightness, scenes, copying, backups and connecting; also as a docker image
- `sdk/aurora`: a Go client for the whole documented API, standard library only
