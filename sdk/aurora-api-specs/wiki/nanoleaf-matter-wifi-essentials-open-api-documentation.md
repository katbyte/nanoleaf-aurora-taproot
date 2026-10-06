<!-- Saved from https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/2296381472, version 37 of 2026-04-01. The .html beside this is the original. -->

# Nanoleaf Matter WiFi Essentials Open API Documentation

- Introduction
- Discovery
- Authorization
- Endpoints
  - Device Information
  - State
    - ON State
    - Brightness
    - Hue
    - Saturation
    - Color Temperature
    - Color Mode
  - Effects
    - The Effect JSON structure
      - animName
      - animType
      - pluginType
      - pluginUuid
      - pluginOptions
      - hexPalette
    - Activating an effect
    - Command API
      - Request All
      - Request
      - Add
      - Display
      - Delete
      - Request Plugins
  - Length
  - Stream Control
    - Addressing LEDs
    - Stream Control Frame Format
- Motion Appendix

# Introduction

The Nanoleaf Matter Wifi Essentials devices expose a simple HTTP REST API over WiFI.

The API is very similar to the [Light Panels series of products API](https://forum.nanoleaf.me/docs), however, there are a few differences.

This API is available on firmware versions above **v3.0.10**and only on the following devices

| **Full Product Name** | **Model Number** |
|---|---|
| Nanoleaf Essentials Holiday String Lights (2023 version) | NL71K1 |
| Nanoleaf Essentials Holiday String Lights (2024 version) | NL71K2 |
| Nanoleaf Essentials Indoor HD Lightstrip | NL72K1 |
| Nanoleaf Essentials Indoor Lightstrip | NL72K3 |
| Nanoleaf Floor Lamp | NL72K4 |
| Nanoleaf Rope Lights | NL72K6 |
| Nanoleaf Essentials Outdoor String Lights | NL73K1 |
| Nanoleaf Essentials Permanent Outdoor Lights | NL73K3 |
| Nanoleaf Essentials WiFi A19 | NL75K1 |

# Discovery

The devices will broadcast the API service on mDNS under the service name : `_nanoleafapi._tcp`. Under this the device will broadcast, the address and port the service is reachable at, the service instance name and the firmware version of the device.

# Authorization

In order to communicate with the device using the API, the user will be required to authenticate with the device. This is done by opening the pairing window on the device.

This can only be done via the Nanoleaf Smarter Series app, under device settings and clicking the **Connect to API**button

Once the button is clicked, the device activates a 30s pairing window during which the user should send a `POST` request to the `/api/v1/new` endpoint. The controller will then respond back with an **authentication token** which should be saved safely as it will be required for all other requests made over the API

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| POST | `/api/v1/new` |  | Success:<br>- 204 No Content<br>Error:<br>- 403 Forbidden | <pre>{<br>    "auth_token": "ppGL6lMTx6bjC3Lri3VLWyNDEh8olxk5"<br>}</pre> |

# Endpoints

The following is a list of endpoints that are available on the Nanoleaf Matter Wifi Essentials devices.

## Device Information

The Device Information endpoint provides information about the device

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/` |  | Success:<br>- 204 No Content<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "name": "Nanoleaf IMLS 2E7",<br>    "serialNo": "V24250ND0003D",<br>    "manufacturer": "Nanoleaf",<br>    "firmwareVersion": "3.0.10",<br>    "hardwareVersion": "1.1.0",<br>    "model": "NL72K1"<br>    "state": {<br>        "brightness": {<br>            "value": 55,<br>            "max": 100,<br>            "min": 0<br>        },<br>        "colorMode": "ct",<br>        "ct": {<br>            "value": 6407,<br>            "max": 6500,<br>            "min": 1200<br>        },<br>        "hue": {<br>            "value": 38,<br>            "max": 360,<br>            "min": 0<br>        },<br>        "on": {<br>            "value": false<br>        },<br>        "sat": {<br>            "value": 3,<br>            "max": 100,<br>            "min": 0<br>        }<br>    }<br>}</pre> |

## State

The state endpoint allows the user to query or modify the state of the device, including changing on/off state, brightness, and colour

### ON State

The `on` endpoint allows the user to modify the on state of the device.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/on` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "on":{<br>        "value":true<br>    }<br>}</pre> |
| PUT | `/api/v1/<auth token>/state` | <pre>{<br>    "on":{<br>        "value":&lt;on&gt;<br>    }<br>}</pre><br>Valid values:<br>`true`<br>/<br>`false` | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Brightness

The `brightness` endpoint allows the user to modify the global brightness of the device. This will affect the brightness of any display the device is currently showing including colours, scenes and screen mirror.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/brightness` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "brightness":{<br>        "value":75,<br>        "max": 100,<br>        "min": 1<br>    }<br>}</pre> |
| PUT | `/api/v1/<auth token>/state` | <pre>{<br>    "brightness":{<br>        "value":&lt;brightness&gt;<br>    }<br>}</pre><br>valid values : As indicated by the<br>`max`<br>and<br>`min`<br>fields in the GET response | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Hue

The `hue` endpoint allows the user to modify the hue value of the colour of the device. Changing this endpoint will place the device into colour mode and will stop screen mirror or any other effect that was running before

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/hue` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "hue":{<br>        "value":180,<br>        "max": 360,<br>        "min": 0<br>    }<br>}</pre> |
| PUT | `/api/v1/<auth token>/state` | <pre>{<br>    "hue":{<br>        "value":&lt;hue&gt;<br>    }<br>}</pre><br>valid values : As indicated by the<br>`max`<br>and<br>`min`<br>fields in the GET response | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Saturation

The `saturation` endpoint allows the user to modify the saturation value of the color of the device. Changing this endpoint will place the device into colour mode and will stop screen mirror or any other effect that was running before

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/sat` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "sat":{<br>        "value":100,<br>        "max": 100,<br>        "min": 0<br>    }<br>}</pre> |
| PUT | `/api/v1/<auth token>/state` | <pre>{<br>    "sat":{<br>        "value":&lt;saturation&gt;<br>    }<br>}</pre><br>valid values : As indicated by the<br>`max`<br>and<br>`min`<br>fields in the GET response | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Color Temperature

The `ct` endpoint allows the user to modify the color temperature of the device. Changing this endpoint will place the device into ct mode and will stop screen mirror or any other effect that was running before

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/ct` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "ct":{<br>        "value":true,<br>        "max": 6535,<br>        "min": 2127<br>    }<br>}</pre> |
| PUT | `/api/v1/<auth token>/state/ct` | <pre>{<br>    "on":{<br>        "value":&lt;color temperature&gt;<br>    }<br>}</pre><br>valid values : As indicated by the<br>`max`<br>and<br>`min`<br>fields in the GET response | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Color Mode

|  |  |
|---|---|
| `hs` | The color of the device is set from the Hue / Saturation values |
| `ct` | The color of the device is set from the Color Temperature |
| `effect` | The color of the device is being controller by a scene |

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/state/colorMode` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>  "colorMode": "effect"<br>}</pre> |

## Effects

The `/effects/` endpoint allows the user to activate, query and modify scenes that are stored on the controller.

### The Effect JSON structure

All effects follow the same JSON structure as given below

```json
{
    "animName": "Northern Lights",
    "animType": "plugin",
    "pluginType": "color",
    "pluginUuid": "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",
    "hexPalette": [
        "002EFF",
        "00EDFF",
        "00FF0C",
        "EDFF00",
        "FF8300",
        "FF0000",
        "FF00D8"
    ],
    "pluginOptions": [
        {
            "name": "linDirection",
            "value": "right"
        },
        {
            "name": "loop",
            "value": true
        },
        {
            "name": "nColorsPerFrame",
            "value": 6
        },
        {
            "name": "transTime",
            "value": 24
        }
    ]
}
```

the word `plugin` and `motion` in this section are equivalent in meaning. `motion` is simply the user facing term for `plugin` with respect to effects

Similarly the word `animation`, `effects` and `scenes` are also interchangeable in text.   
The controller however will use the word `animation` in all of its API commands and fields

#### animName

The `animName` field indicates the unique name of the effect on the device. Its a unique identifier for the effect and cannot be duplicated or used by any other effect

#### animType

The `animType` field indicates the type of effect stored on the controller.

Currently, there is only one kind of `animType` that can be stored on the controller : `plugin`.

`plugin` effects create an effect by selecting a motion from the list of motions that are stored on the controller. See: [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Motion-Appendix)

There are two kinds of animTypes that are available on the Nanoleaf Matter WiFi Essentials:

1. `plugin` create an effect by selecting a motion from the list of motions that are stored on the controller. See: [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Motion-Appendix)
2. `extControl` which sets the device into stream control mode for use with the DSK app, 4D or with [stream control](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Stream-Control)

#### pluginType

The `pluginType` indicates the kind of motion that is in use by this effect. There are two possible options:

1. `rhythm`: The motion depends on input from the device microphone to render colors on the screen
2. `color`: The motion is a simple dynamic motion that simply uses an algorithm to render colors on the screen.

#### pluginUuid

This is the `uuid` of the motion that the effect is currently using.

A list of the uuids of all the motions available are provided in the [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Motion-Appendix)

#### pluginOptions

This is a JSON object representing the plugin options that the effect currently sets. A list of plugin options available for each motion is available in the [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Motion-Appendix)

#### hexPalette

This is a JSON array representing the colors that are used by this effect. The colors are encoded as hex triplets with each 2B hexadecimal digit representing a color channel.

Examples:

| **Hex triplet** | **Color** |
|---|---|
| #FF0000 | R = 255<br>G = 0<br>B = 0 |
| #00FF80 | R = 0<br>G = 255<br>B = 180 |
| #4080FF | R = 64<br>G = 128<br>B = 255 |

### Activating an effect

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/effects/select` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>&lt;name of effect&gt;</pre> |
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "select":&lt;name of effect&gt;<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

### Command API

The Command API allows the user to perform various actions on the controller related to scenes, such as adding / displaying and deleting effects.

All command API requests must be wrapped in the `write` object in JSON. This is shown in the examples as well.

#### Request All

Allows the user to get a list of all the effects and their properties from the controller

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write":{<br>    "command":"requestAll"<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized | <pre>{<br>    "animations": [<br>        {<br>            "animName": "Cozy Glow",<br>            "animType": "plugin",<br>            "pluginType": "color",<br>            "pluginUuid": "0794a6b3-f6cc-452d-a900-30081604b7ec",<br>            "hexPalette": [<br>                "FCDEB8",<br>                "F7D299"<br>            ],<br>            "pluginOptions": []<br>        },<br>        {<br>            "animName": "Dark Side",<br>            "animType": "plugin",<br>            "pluginType": "color",<br>            "pluginUuid": "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",<br>            "hexPalette": [<br>                "120901",<br>                "120901",<br>                "120901",<br>                "780000",<br>                "FF0000"<br>            ],<br>            "pluginOptions": [<br>                {<br>                    "name": "nColorsPerFrame",<br>                    "value": 40<br>                }<br>            ]<br>        }<br>    ]<br>}</pre> |

#### Request

The request API allows the user to query an effect by its name specifically. Note that if the effect does not exist the controller will throw an error.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write":{<br>    "command":"request",<br>    "animName":"Dark Side"<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request if the effect does not exist<br>- 401 Unauthorized | <pre>{<br>    "animName": "Dark Side",<br>    "animType": "plugin",<br>    "pluginType": "color",<br>    "pluginUuid": "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",<br>    "hexPalette": [<br>        "120901",<br>        "120901",<br>        "120901",<br>        "780000",<br>        "FF0000"<br>    ],<br>    "pluginOptions": [<br>        {<br>            "name": "nColorsPerFrame",<br>            "value": 40<br>        }<br>    ]<br>}</pre> |

#### Add

The add API enables users to add an effect, which is then saved on the device and appears in the effects list. Note that the fields required in `pluginOptions` object are dependent on the plugin that is used. For more information about which plugin uses what options see here : [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/2298052616/Motion+Appendix)

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write": {<br>    "command": "add",<br>    "animType": "plugin",<br>    "animName": "Dark Side",<br>    "colorType": "HSB",<br>    "hexPalette": [<br>        "120901",<br>        "120901",<br>        "120901",<br>        "780000",<br>        "FF0000"<br>    ],<br>    "pluginType": "color",<br>    "pluginUuid": "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",<br>    "pluginOptions": [<br>      {<br>        "name": "linDirection",<br>        "value": "left"<br>      },<br>      {<br>        "name": "loop",<br>        "value": true<br>      },<br>      {<br>        "name": "nColorsPerFrame",<br>        "value": 4<br>      },<br>      {<br>        "name": "transTime",<br>        "value": 24<br>      }<br>    ]<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

#### Display

The display API allows users to play a temporary effect on the device, which is not saved. Note that the fields required in `pluginOptions` object are dependent on the plugin that is used. For more information about which plugin uses what options see here : [Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/2298052616/Motion+Appendix)

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write": {<br>    "command": "display",<br>    "animType": "plugin",<br>    "animName": "Dark Side",<br>    "colorType": "HSB",<br>    "hexPalette": [<br>        "120901",<br>        "120901",<br>        "120901",<br>        "780000",<br>        "FF0000"<br>    ],<br>    "pluginType": "color",<br>    "pluginUuid": "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",<br>    "pluginOptions": [<br>      {<br>        "name": "linDirection",<br>        "value": "left"<br>      },<br>      {<br>        "name": "loop",<br>        "value": true<br>      },<br>      {<br>        "name": "nColorsPerFrame",<br>        "value": 4<br>      },<br>      {<br>        "name": "transTime",<br>        "value": 24<br>      }<br>    ]<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request<br>- 401 Unauthorized |  |

#### Delete

The delete API enables users to remove an effect from the device. Once deleted, the effect cannot be recovered.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write":{<br>    "command":"delete",<br>    "animName":"Dark Side"<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request if the effect does not exist |  |

#### Request Plugins

The request plugins API allows the user to query the list of motions that are available on the device. This list of motions can be different for different devices and can change with a firmware update. If the controller was to change the motion list, it will figure out a migration strategy for effects that are using a motion that will be removed.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>  "write":{<br>    "command":"requestPlugins",<br>  }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request if the effect does not exist<br>- 401 Unauthorized | <pre>[<br>    "6970681a-20b5-4c5e-8813-bdaebc4ee4fa",<br>    "027842e4-e1d6-4a4c-a731-be74a1ebd4cf",<br>    "b3fd723a-aae8-4c99-bf2b-087159e0ef53",<br>    "1dab05d3-07bf-4648-9d48-db8dd196ed28",<br>    "ba632d3e-9c2b-4413-a965-510c839b3f71",<br>    "70b7c636-6bf8-491f-89c1-f4103508d642",<br>    "fe0d0ba8-741f-4210-940c-454eceed010f",<br>    "e8b69f65-df13-480b-a949-db1bbc9a1cde",<br>    "30a017d4-e6a5-4647-8c41-5a7cd38ff907",<br>    "713518c1-d560-47db-8991-de780af71d1e",<br>    "0794a6b3-f6cc-452d-a900-30081604b7ec",<br>    "e2eb1818-07ac-495f-a10d-7c192d7df705",<br>    "5c4d343f-8eca-47e9-8c01-92662dff4eb6",<br>    "29929fc2-6905-41c0-a7d7-1582a8c11f17",<br>    "db8ce916-88de-484f-bdbb-afd732053105",<br>    "4eb1454c-ab57-40ad-a51c-f248bc9aae40",<br>    "27477952-a5f7-42cc-b38b-bde3bb8c9929",<br>    "0349fc36-7727-4ebc-bc1b-a0021c55038e",<br>    "fe605997-2b21-4990-8d46-cde0610d9deb",<br>    "337784c0-6c98-447a-8a9f-fc6ac44a8024",<br>    "8f32ae5c-1a73-4170-aeba-80735b8e8eda",<br>    "af555dc5-67de-4b63-9674-85fefc663dee"<br>]</pre> |

## Length

The length endpoint allows the user to query the number of LEDs on the Lightstrip that is connected to the device. Note that the device will only detect the length once on startup, and if the length of the LS is modified by cutting it, for instance, the controller must be power cycled for it to detect the length of the Lightstrip once again.

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| GET | `/api/v1/<auth token>/length` |  | Success:<br>- 200 OK<br>Error:<br>- 401 Unauthorized | <pre>{<br>    "numLEDs": 300<br>}</pre> |

## Stream Control

Stream Control allows the user to take control of every single LED in the Light Strip or String Lights thats connected to the controller.

Stream Control Data Frames are sent to the controller over UDP to the IP address of the controller and Port `60222`

To activate Stream Control mode the effects API can be used:

| **VERB** | **ENDPOINT** | **REQUEST BODY** | **RESPONSE** | **RESPONSE BODY** |
|---|---|---|---|---|
| PUT | `/api/v1/<auth token>/effects` | <pre>{<br>    "write": {<br>        "command": "display",<br>        "animType": "extControl",<br>        "extControlVersion": "v2"<br>    }<br>}</pre> | Success:<br>- 204 No Content<br>Error:<br>- 400 Bad Request if the effect does not exist<br>- 401 Unauthorized |  |

### Addressing LEDs

The LEDs are addressed simply by their index from 0 - N, where N is the total number of LEDs, i.e. the `id` of each LED is the same as its index. The typical Addressable LS has 300 LEDs and for the Holiday String Lights its 250, but for the Outdoor String Lights you might have to count the number you have.

In case you are using a WiFi A19, which is a single point source of light, the Number of LEDs for such a device will be simply `1`

### Stream Control Frame Format

Each Stream Control Data Frame sent to the controller is formatted as follows:

|  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| **Field** | Num Panels | PanelId0 | R0 | G0 | B0 | W0 | Transition Time0 | … | PanelId N | R N | G N | B N | W N | Transition Time1 |
| **Size** | 2B | 2B | 1B | 1B | 1B | 1B | 2B | … | 2B | 1B | 1B | 1B | 1B | 2B |

where N is the number of LEDs that are being addressed in the above frame. Its not necessary to update all LEDs in the same frame, any number and any subset of LEDs may be addressed

All multi bytes fields are encoded as big endian

Transition time is represented as multiples of 100ms. So a value of 5 would mean a transition time of 500ms

The White LED field (`Wn`) is always set to 0

Example:

Say the following LEDs with ids are to be set to the following colors, all with transition times of 500ms

| **LED id** | **LED color** |
|---|---|
| 128 | Red - #FF0000 |
| 23 | Magenta - #FF00FF |
| 282 | Lime - #80FF00 |

The stream control data frame would like as follows, bytes represented as hexdecimal numbers

|  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |  |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 00 | 04 | 00 | 80 | 00 | FF | FF | 00 | 00 | 05 | 00 | 17 | FF | 00 | FF | 00 | 00 | 05 | 01 | 1A | 80 | FF | 00 | 00 | 00 | 05 |

# Motion Appendix

This page describes the motions and its properties and parameters that are provided by the Nanoleaf Matter Wifi Essentials devices.

Note that not all motions will be present on all devices. Devices may only have a subset of motions.

To get the list of motions stored on a device, see: [https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Request-motions](https://nanoleaf.atlassian.net/wiki/spaces/NOAD1/pages/2296381472/Nanoleaf+Matter+WiFi+Essentials+Open+API+Documentation#Request-motions)

[Motion Appendix](https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/2298052616/Motion+Appendix#Motions)
