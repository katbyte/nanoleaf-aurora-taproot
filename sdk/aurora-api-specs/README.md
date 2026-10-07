# aurora-api-specs

Nanoleaf's documentation of the local API, saved here, and what real controllers do that it does not say.

Nanoleaf publishes no machine-readable description of the API: no OpenAPI document, only pages on a wiki. So
`sdk/aurora` is written by hand against these saved copies, and two things keep it honest:

- **`make apicheck`** reads every endpoint and every effect command out of the saved page and fails if `sdk/aurora`
  has no method for one. Today that is 27 of 27 endpoints and 8 of 8 commands.
- **The acceptance tests** replay what a real controller answered (`acceptance/testdata/cassettes`), and
  `make record-check` compares those recordings with a live controller, so an answer that firmware changes is found.

The documentation says what Nanoleaf meant. The controller is what is true; where they differ, the controller wins
and the difference is written down below.

## What is saved

| Path | What | From |
|---|---|---|
| `wiki/nanoleaf-light-panels-open-api-documentation.*` | the API reference `sdk/aurora` is written against | the official wiki |
| `wiki/*.html`, `wiki/*.md` | every other page of the wiki's API space: the Matter WiFi Essentials API, the motion appendix, the USB lightstrip protocol, the list of cloud services | the official wiki |
| `wiki/attachments/` | the pages' images | the official wiki |
| `mirror/gist-docs.md` | the older documentation from Nanoleaf's developer forum, as somebody kept it | [a gist by dennishn](https://gist.github.com/dennishn/ae06bf574748727bcce5127394a8ba43) |
| `sources.json` | each file's address, version, date and checksum | written by the refresh |

Each wiki page is saved twice: the `.html` is the page as the wiki renders it, and the `.md` is the same page
converted for reading and for `grep`. The `.html` is the original.

**`make spec-refresh`** saves everything afresh (`scripts/spec-refresh.py`). Review the diff afterwards: a changed
page is a changed API, and `make apicheck` then says whether the SDK still covers it.

The forum address most client libraries point to, `forum.nanoleaf.me/docs/openapi`, no longer works: it redirects
to Nanoleaf's support site behind a bot check, and the Internet Archive holds no copy of it. The gist is the only
copy of that version found. It is older than the wiki, and clearer in places, because the wiki's rendering has
eaten everything the forum wrote in angle brackets: the token in every path (`/api/v1//state` on the wiki is
`/api/v1/<auth_token>/state`), and the placeholders in the event stream's examples.

## Where the documentation and a controller differ

Checked against an NL22 on firmware 5.2.1, 6 October 2026, with reads only, except where another firmware is named.

**Answers the documentation does not list.** A controller answers for the whole of a section as well as for each
value in it: `GET /state` and `GET /rhythm` work, and are in the SDK as `State` and `Rhythm`. The whole answer
(`GET /`) also carries sections the documentation's example lacks: `hardwareVersion`, `cloudHash`, `discovery`,
`firmwareUpgrade` and `schedules`. The SDK keeps them in `Info.Raw`.

**Scenes.** The documentation describes effects by the name of their motion (`animType` of `wheel`, `flow`,
`random` and so on, each with its own fields). Every one of the 17 scenes on the controller is `animType: plugin`
instead, naming its motion by `pluginUuid` with its settings in `pluginOptions`, and carries a `hasOverlay` field
the documentation never mentions. Their `version` is `"1.0"` for some and `"2.0"` for others. This is why
`sdk/aurora` keeps a scene as the controller's own document and never rebuilds one.

**`palette`, not `Palette`.** The documentation's examples for adding an effect spell it with a capital. Controllers
send it in lower case. The SDK reads either.

**`sideLength`.** The documentation says firmware from 5.0.0 reports 0 and the length must be worked out from each
panel's shape. This controller, on 5.2.1, reports 150.

**`requestPlugins`.** Documented with `"version": "2.0"`. It answers the same with or without it. The SDK sends it
as documented.

**Refusals.** As documented, and confirmed: a token request when nobody has held the button is `403` with nothing in
it; any request under a token the controller does not know is `401`; a path it does not serve, and a request for an
effect it does not hold, are `404` with nothing in it.

**Events.** The stream opens on an NL22 (`200`, `text/event-stream`) and stays open.

**More sections than are documented.** On firmware 5.3.2, `GET /discovery`, `/schedules`, `/cloudHash` and
`/firmwareUpgrade` each answer with their own section, as `/effects` and `/panelLayout` do. The values that
identify the controller do not: `GET /name`, `/serialNo`, `/model`, `/firmwareVersion`, `/manufacturer` and
`/hardwareVersion` are all `404`.

**The name a controller gives itself cannot be set.** Nothing documented sets it, and the obvious guesses are
refused. On an NL22 on firmware 5.3.2, 6 October 2026, each of these answered `404` with nothing in it and left
the name as it was: `PUT /` with `{"name": "..."}` and with `{"name": {"value": "..."}}`, and `PUT /name` with
`{"name": "..."}` and with `{"value": "..."}`. The documentation's one mention of the name changing is that it
"could be updated by an iOS user using WAC", Apple's Wi-Fi setup for accessories. `taproot rename` therefore only
changes taproot's own name for a controller.

**Firmware 5.3.2 adds a field to every scene it stores.** A scene read from firmware 5.2.1 and added to a
controller on 5.3.2, 6 October 2026, was accepted as it was and plays. Read back, it is the same in every field,
with one more: `"rhythmFeatureSource": 1`. Every scene on a 5.3.2 controller has it, the built-in ones included, and
no scene on 5.2.1 does. So it is the firmware's own bookkeeping and not part of the scene, and `sdk/aurora` compares
scenes without it (`Effect.SameScene`): otherwise a scene copied from older firmware would look different from its
own copy, and could never be found to be there already.

## What the desktop app sends that the documentation does not list

Read out of Nanoleaf Desktop 3.0.1 (an Electron app; its JavaScript ships as text in `app.asar`), 6 October 2026.
This is what the app sends, not what a controller has been seen to answer: none of it has been sent to a controller
by taproot yet, and the app gates some of it by model, so what an NL22 does with each is still to be checked. All of
it is in `sdk/aurora/app.go`, and `taproot firmware`, `scene paint` and `set buttons|fade|recovery` use it; the canned
controller the tests use answers each in the obvious way until a real one has been recorded.

**Firmware.** The app never downloads a firmware file for a Light Panels controller: it asks the controller to fetch
and install one itself, from Nanoleaf's cloud. Two undocumented endpoints under the token:

- `GET /firmwareUpgrade` answers `{"firmwareAvailability": bool, "newFirmwareVersion": "x.y.z" | null}`, and the same
  section is in `GET /`. The app shows the version and an update button when `firmwareAvailability` is true.
- `PUT /firmwareUpgrade` with `{"command": "triggerFirmwareUpgrade"}` starts it. The body is not wrapped in `write`
  as effect commands are. The app then reads `GET /` every 10 seconds until `firmwareAvailability` is false again.

Confirmed on a real controller, 7 October 2026: the NL22 on 5.2.1 answered `GET /firmwareUpgrade` with
`{"firmwareAvailability": true, "newFirmwareVersion": "5.3.2"}`, the shape the app reads. Earlier the same day all
three controllers answered `{}`, the one on 5.2.1 included, so `{}` does not mean "up to date": it means the
controller has not heard from the cloud yet. Asking is a read; nothing was triggered. The `firmwareUpgrade` section
of `GET /` stayed `{}` on the same controller at the same time: the whole answer does not carry it, and only the
path of its own says it. Where the cloud keeps the files is not in the app; other product
lines' files are on public S3 buckets (`canvas-firmware`, `hexagon-firmware`, `nl52-firmware`, `nl59-firmware`,
`<version>.firmware`), and no bucket of any obvious name exists for Light Panels. The offline route from community
notes, holding the power button until the LEDs run and then uploading a file to `http://192.168.2.1/` on the
controller's own network, is the "Local Firmware Update, TCP 80" in Nanoleaf's services list; that port is closed in
normal running.

**What an upgrade looks like**, from the NL22 going from 5.2.1 to 5.3.2 on 6 October 2026, watched every four
seconds: the trigger was taken at once (`204`); within seconds the controller stopped answering anything, `GET /`
included, and stayed silent for about two and a half minutes; then it answered again on 5.3.2 with
`firmwareUpgrade` back to `{"firmwareAvailability": false, "newFirmwareVersion": null}`, running the scene it had
been running. Its own scene came through byte for byte, with the `rhythmFeatureSource` field 5.3.2 adds. One of
the stock scenes (`Color Burst`) was gone afterwards, though a controller shipped on 5.3.2 has it: whatever the
upgrade does to the built-in scenes, it is not nothing, and the backup taken first is what gets it back.

**Commands to `PUT /effects`** (each as `{"write": {"command": ..., ...}}`) that the documentation does not list:

| Command | With | What the app uses it for |
|---|---|---|
| `enableAllControllerButtons`, `disableAllControllerButtons` | | locking the buttons on the controller |
| `enableSceneChangeAnimation`, `disableSceneChangeAnimation` | | the fade between scenes |
| `setPLRConfig` | `"PLRConfig": bool` | whether it comes back on after a power cut |
| `getShortIdMap`, `getAdjacencyData` | | which panel touches which, for the layout editor |
| `displayOverlay` | `"overlayPalette"`, `"animData"` | drawing on top of the running scene |
| `display` | `"animType": "static"`, `"animData"`, `"palette": []` | a colour per panel, unsaved |
| `requestBrightnessSensorConfig`, `setBrightnessSensorConfig` | `"brightnessSensorConfig"` | auto-brightness, on models with the sensor |
| `requestTouchConfig`, `configureTouch`, `getTouchKillSwitch`, `setTouchKillSwitch` | `"touchConfig"`, `"touchKillSwitchOn"` | touch, on models that have it (not NL22) |

Those are the only paths the app uses under the token: `/effects`, `/events`, `/firmwareUpgrade`, `GET /` for
everything, and `DELETE` of the token. It reads `cloudHash` from `GET /` to tie the controller to an account.

**The MAC address.** Nothing the controller serves says it in full. The name it gives itself carries the last three
octets (`Light Panels 53:A6:3C`), and the first three are Nanoleaf's: IEEE's registry gives `00:55:DA:5x:xx:xx` to
Nanoleaf, and every controller here is named `5x:..`. So `taproot list` shows `00:55:DA:` and the name's three. The
`id` in a controller's mDNS announcement is shaped like a MAC address and is not one: it changes when the controller
is reset.

## Not yet checked on a controller

The canned controller the tests use (`sdk/aurora/auroratest`) does what the documentation says for these, and each
is marked to be checked the first time a write is recorded:

- what `add`, `delete` and `rename` answer when they work (`204` is assumed, as documented; `add` is known to work)
- what a controller does when the scene that is running is deleted, or renamed; taproot refuses to try either
- whether firmware 5.2.1 keeps or drops `rhythmFeatureSource` when it is given a scene that has it
- what a controller says to a scene naming a plugin it does not have; taproot refuses to send one
- which of the commands the desktop app sends (above) an NL22 answers, and what `firmwareUpgrade` holds while an
  upgrade runs
