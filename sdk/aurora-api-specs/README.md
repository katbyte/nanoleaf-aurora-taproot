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

Checked against an NL22 on firmware 5.2.1, 6 October 2026, with reads only.

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

## Not yet checked on a controller

Nothing has been written to a real controller yet. The canned controller the tests use (`sdk/aurora/auroratest`)
does what the documentation says for these, and each is marked to be checked the first time a write is recorded:

- what `add`, `delete` and `rename` answer when they work (`204` is assumed, as documented)
- whether a controller stores a scene exactly as it was sent, or writes it its own way; taproot reads every scene
  back after writing it and reports any field that differs
- whether a scene read from firmware 5.2.1 is accepted as it is by firmware 5.3.2
- what a controller says to a scene naming a plugin it does not have; taproot refuses to send one
