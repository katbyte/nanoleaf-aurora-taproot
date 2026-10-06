 

# 1\. Introduction

This documentation provides the detailed interfaces and structure of the Nanoleaf Devices OpenAPI, an interface developed for third-party developers to authenticate, control, and obtain information of the Panels, via a simple RESTful API over HTTP.

# 2\. Definitions

|     |     |
| --- | --- |
| Term | Definition |
| User | Any application intended to control or obtain data from panels. Examples: a third-party mobile app, web-based interface for testing, Postman |
| Panels/Device | Refers to Nanoleaf Light panels and Canvas Light Squares |
| Effect/Animation | A collection of user presets that define panels operating mode that can be activated through the Nanoleaf App, Siri, HomeKit, Amazon Alexa, IFTTT, or any 3rd party implementing the Open API. Nanoleaf uses "Effect" or "Scene" for all user-facing references, although "animation" is used in certain parts of the API. |
| Palette | A collection of colors used to define an Effect |
| Plugin/Motion | An Effect type. Also referred to as "Scene Types", they instruct Panels how to render light. Nanoleaf uses "Motions" for all user-facing references. "Plugin" is mostly used in the API. |
| Options | Plugin options that modify how the effect should be rendered |
| Scene | Often used synonymously with Effect. A Scene is a more general purpose container that can control multiple smart home devices, especially in the context of HomeKit. For the purposes of the panels API, a Scene and Effect are synonymous. |
| Controller/ Control Square | This is the hardware responsible for controlling Light Panels/ Light Squares and relaying messages to and from the local wifi network. |

# 3\. Functional Principles

## 3.1. Color Modes

Panels have 3 primary color modes:  
**Effect mode (effect):** this is the main operating mode. It includes static and dynamic Effects  
**Hue/Saturation mode (hs):** this is a simple color mode where Panels act as a normal color light bulb with hue/saturation defining color  
**Color temperature mode (ct):** this is a simple color mode where Panels act as a normal color light bulb with CCT (in Kelvin) defining color. The acceptable range is 1200-6500 for Nanoleaf Lines, Nanoleaf Shapes, Nanoleaf Canvas and Nanoleaf Light Panels. The acceptable range for Nanoleaf Elements is 1500-4000

## 3.2. Effects

Effects describe instances of user-saved presets. These are made of 3 main components:

1.  Palette
2.  Plugins
3.  Options

### 3.2.1. Palettes

A palette defines the colors that are to be used in a given effect. This can be used for static type Effects, where a user would use a select-and-tap methodology to create a static effect of their liking. For dynamic effects the palette will define the colors used to generate the effect. Palette size should be limited to 20 colors.

### 3.2.2. Plugins

Motions are essentially a set of instructions which instruct the panels on how to output light based on inputs from Touch (Canvas only), Sound and the passage of time. A motion may use any combination of the aforementioned inputs and render the light on the panels.

Motions can be designed, built and submitted to Nanoleaf using the Nanoleaf SDK (Nanoleaf Light Panels only for now) and are implemented in C++. Once approved by Nanoleaf, these plugins are then available to be downloaded by Nanoleaf users.

Since motions act like plugins in software, they are also called plugins and the terminology in the context of this document and application is interchangeable.

Every Motion has a uuid which is its unique identifier. An effect must indicate which plugin it uses through its uuid.

When using plugins, the animType in the effect json is set to 'plugin'.

Default legacy colour effects such as flow, wheel etc have also been converted to Motions and have a fixed uuid. When activating effects that use these default animTypes, the controller automatically maps the effect to the corresponding Motion's uuid. Below is a list of default plugins available on the controller out-of-the box. The mapping for the uuid of the motion and the default legacy effect type is provided in the following table.  
  
**Default Plugins**

|     |     |     |
| --- | --- | --- |
| **Plugin** | **Description** | **Options** |
| Random | Panels randomly transition between colors and brightnesses | Transition time, delay time |
| Flow\* | 2+ chosen colors gradually mix into each other as if flowing paint is mixing | transTime, delay time, loop, LinDirection |
| Wheel\* | Color gradient cycles across panels in a user specified direction | transTime, loop, LinDirection |
| Highlight | Subset of random effect where a primary color dominates the transitions and 1 or more secondary colors occasionally transition in and out | transTime, delay time, MainColorProb |
| Fade | Colors cycle across panels in sync | transTime, delay time, loop |
| Explode | Similar to flow, but the colors move out from the centre of the panel system, rather than the edge | transTime, delay time, loop |

\* The difference between flow and wheel is subtle. Flow provides full transition between discrete colors with a gradient-type transition; the end of the transition will be when panels are a single color. Wheel provides a constant transition between a set of colors, meaning that there is always a gradient of colors displayed.  
  
**Legacy Plugin UUID Mapping**

|     |     |
| --- | --- |
| Wheel | 6970681a-20b5-4c5e-8813-bdaebc4ee4fa |
| Flow | 027842e4-e1d6-4a4c-a731-be74a1ebd4cf |
| Explode | 713518c1-d560-47db-8991-de780af71d1e |
| Fade | b3fd723a-aae8-4c99-bf2b-087159e0ef53 |
| Random | ba632d3e-9c2b-4413-a965-510c839b3f71 |
| Highlight | 70b7c636-6bf8-491f-89c1-f4103508d642 |

### 

### 3.2.3. Plugin Options

Plugins can use any of the Nanoleaf-approved option types to further control how panels render light.

|     |     |     |     |
| --- | --- | --- | --- |
| **Name** | **Type** | **Global Limits** | **Comment** |
| transTime | int | 1-600 | Generally, the time it takes to go from one palette colour to another, in tenths of a second. |
| loop | bool | true/false | Indicates whether an animation should loop or not |
| linDirection | string | left, right, up, down | Linear direction, based on user‚Äôs global orientation |
| radDirection | string | in, out | Radial direction, based on layout center |
| rotDirection | string | cw, ccw | Circular Direction, around the layout center. cw = clockwise; ccw = counter-clockwise |
| delayTime | int | 0-600 | Indicates how long the plugin will dwell on a palette colour, in tenths of a second. |
| nColorsPerFrame | int | 1-50 | Modifier that indicates how much of a palette is shown on the layout. 50 is a global limit, but generally the max will be dynamically set by the App to the size of the user‚Äôs palette. |
| mainColorProb | double | 0.0-100.0 | Probability of background colour being used |

### 

### 3.2.4. Effects Commands

Commands are sent to Panels Write endpoint using a structure outlined in the next sections. If a command response is warranted, the controller will include it in the response.

Command Type is a mandatory field and is always present as the first parameter of the command data string. There are 7 different types of Command Types:

*   Add
*   Delete
*   Request
*   Request All
*   Request Plugins
*   Display
*   Display Temp

**Effects Commands Structure**  
The following key-value pairs are included.

|     |     |     |     |
| --- | --- | --- | --- |
| **Key** | **Value Type** | **Required** | **Description** |
| command | string | Yes | Command type: add, request, delete, display, rename, requestAll |
| version | string | Yes | Command version: 1.0 |
| duration | number | no  | Sets duration of temporary Effect in seconds |
| animName | string | no  | effect Name. Not required for Display commands |
| newName | string | no  | Used for renaming an effect |
| animType | string | Yes | effect type: random, flow, wheel, fade, highlight, custom, static |
| animData | string | No  | Rendered effect data: only needed for custom and static types |
| colorType | string | no  | Defines colour type for palette (should be HSB). Required for Add and Display commands. |
| palette | array | no  | User defined colours for creating all effects except custom types |
| palette::hue | number | no  | Hue (0-359), integer |
| palette::saturation | number | no  | Saturation (0-100) |
| palette::brightness | number | no  | Brightness (0-100) |
| palette::probability | number | no  | Probability of colour appearance in highlight effect type |
| brightnessRange | object | No  | Brightness range for random type effects |
| brightnessRange:: minValue | number | No  |     |
| brightnessRange:: maxValue | number | No  |     |
| transTime | Object or value | no  | Transition time between colours. This will be a single value for flow and wheel, and a range for random, fade, and highlight |
| transTime:: minValue | Number | no  |     |
| transTime::maxValue | Number | no  |     |
| delayTime | Object or value | No  | Dwell time between colours. This will be a single value for flow and wheel, and a range for random, fade, and highlight |
| delayTime:: minValue | Number | No  |     |
| delayTime:: maxValue | Number | No  |     |
| flowFactor | Number | No  | Degree of flow for flow type effects. flowFactor can range from 0 to infinite. |
| explodeFactor | Number | No  | Math factor for explode. Can range from 0.0 to 1.0. 0.5 recommended. |
| windowSize | number | No  | Window size that determines the number of colours shown in a wheel effect |
| direction | string | no  | effect directionality |
| loop | boolean | yes |     |

#### 3.2.4.1. Add/Update Effects

“Add” will add (or update) an effect to the Panels and update the Effect list. If the Effect already exists, issuing this command will overwrite the previous Effect. Examples shown below:  
  
**Adding animation that uses the "flow" plugin**  
 

Copy`{   "command": "add",   "version": "2.0",   "animType": "plugin",   "animName": "My Animation",   "colorType": "HSB",   "pluginUuid": "027842e4-e1d6-4a4c-a731-be74a1ebd4cf",   "pluginType": "color",   "pluginOptions": [     {       "name": "transTime",       "value": 2     },     {       "name": "direction",       "value": "left"     },     {       "name": "loop",       "value": true     }   ],   "Palette": [     {       "hue": 0,       "saturation": 100,       "brightness": 100     },     {       "hue": 120,       "saturation": 100,       "brightness": 100     },     {       "hue": 240,       "saturation": 100,       "brightness": 100     }   ] }`

#### 3.2.4.2. Delete Effects

Copy`{   "command": "delete",   "animName": "My Flow Animation" }`

#### 3.2.4.3. Request Effects

Request command is used to request effect data from the controller for a particular effect.  
 

Copy`{   "command": "request",   "animName": "My Animation" }  Sample Response:  {   "version": "1.0",   "animName": "My Animation",   "animType": "plugin",   "colorType": "HSB",   "palette": [     {       "hue": 235,       "saturation": 100,       "brightness": 51,       "probability": 0     },     {       "hue": 307,       "saturation": 100,       "brightness": 100,       "probability": 0     },     {       "hue": 182,       "saturation": 100,       "brightness": 100,       "probability": 0     },     {       "hue": 62,       "saturation": 100,       "brightness": 100,       "probability": 0     },     {       "hue": 2,       "saturation": 100,       "brightness": 100,       "probability": 0     },     {       "hue": 0,       "saturation": 0,       "brightness": 91,       "probability": 0     }   ],   "pluginType": "rhythm",   "pluginUuid": "fe0d0ba8-741f-4210-940c-454eceed010f" }`

#### 3.2.4.4. Request All

Request All is used to request all the effect data from the controller for all effects stored on it.  
 

Copy`{   "command": "requestAll", }  Sample Response:  {   "animations": [     {       "loop": true,       "version": "1.0",       "animName": "Slow",       "animType": "custom",       "animData": "7 224 7 0 0 0 0 0 0 0 0 255 5 0 0 0 0 5 0 0 0 255 5 0 0          0 0 5 0 0 0 255 5 0 0 0 0 5 89 2 0 0 0 0 0 255 0 0 0 10 210 2 0 0 0          0 0 255 255 0 0 15 116 2 0 0 0 0 0 0 255 0 0 20 173 2 0 0 0 0 0 0 255          255 0 25 126 2 0 0 0 0 0 0 0 255 0 30 81 2 0 0 0 0 0 255 0 255 0 35"     },     {       "loop": true,       "version": "1.0",       "animName": "Flames",       "animType": "highlight",       "colorType": "HSB",       "animData": null,       "palette": [         {           "hue": 30,           "saturation": 100,           "brightness": 100,           "probability": 58         },         {           "hue": 45,           "saturation": 100,           "brightness": 100,           "probability": 8         },         {           "hue": 25,           "saturation": 100,           "brightness": 100,           "probability": 25         },         {           "hue": 40,           "saturation": 100,           "brightness": 100,           "probability": 8         }       ],       "brightnessRange": {         "minValue": 40,         "maxValue": 100       },       "transTime": {         "minValue": 2,         "maxValue": 10       },       "delayTime": {         "minValue": 0,         "maxValue": 0       }     },     {       "delayTime": {         "minValue": 0,         "maxValue": 0       },       "version": "1.0",       "animName": "Lightning",       "animType": "random",       "colorType": "HSB",       "animData": null,       "palette": [         {           "hue": 0,           "saturation": 0,           "brightness": 100         },         {           "hue": 0,           "saturation": 0,           "brightness": 0         }       ],       "brightnessRange": {         "minValue": 25,         "maxValue": 100       },       "transTime": {         "minValue": 0,         "maxValue": 0       }     }   ] }`

#### 3.2.4.5. Rename Effects

Copy`{   "command": "rename",   "animName": "My Flow Animation",   "newName": "My New Flow Animation" }`

#### 3.2.4.6. Display Effects

Display is used to see a preview on Panels. This functions as a temporary display on Panels without saving as an Effect. The command structure is identical to add, but without a required animName field:  
  
_Temporary dynamic effect_

Copy`{   "command": "display",   "version": "2.0",   "animType": "plugin",   "colorType": "HSB",   "pluginUuid": "027842e4-e1d6-4a4c-a731-be74a1ebd4cf",   "pluginType": "color",   "pluginOptions": [     {       "name": "transTime",       "value": 2     },     {       "name": "direction",       "value": "left"     },     {       "name": "loop",       "value": true     }   ],   "Palette": [     {       "hue": 0,       "saturation": 100,       "brightness": 100     },     {       "hue": 120,       "saturation": 100,       "brightness": 100     },     {       "hue": 240,       "saturation": 100,       "brightness": 100     }   ] }`

#### 3.2.4.7. Display Temporary Effects

Display temporary is used to temporarily set a color mode on Panels. This is typically used for temporary notifications or visual indicators. There are two fundamental types with a third derivative type.  
  
Set saved effect:

Copy`{   "command": "displayTemp",   "duration": ,   "animName":  }`

Set arbitrary effect:  
  
You should include the exact animationOptionJson, but the command must be displayTemp and there must be a duration field.

Copy`{   "command": "displayTemp",   "duration": ,   ‚Ä¶‚Ä¶ }`

  
  
Set specific color:  
This is a derivative of an arbitrary effect. As a tip, you can flash the colour by adding a black colour to the palette and using the fade plugin with some defined transition time.

Copy`{   "command": "displayTemp",   "duration": ,   "animType": "solid",   "palette": [     {       "hue": < user >,       "saturation": < user >,       "brightness": < user >     }   ],   "colorType": "HSB" }`

#### 3.2.4.8. Request Plugins

Request command is used to request plugins data from the Panels.  
 

Copy`{   "command": "requestPlugins",   "version": "2.0" }`

  
  
The response contains all the plugins that are present on the controller. The UUID, name, description,author, type, tags, features and pluginConfig data are returned as shown below.  
Sample Response for Nanoleaf Canvas Panels

Copy`{   "plugins": [     {       "uuid": "027842e4-e1d6-4a4c-a731-be74a1ebd4cf",       "name": "Flow",       "description": "Imagine you dump each color onto your panels.          Watch the color flow in a direction of your choosing.",       "author": "Nanoleaf",       "type": "color",       "tags": [         "zen",         " happy"       ],       "features": [         "touch"       ],       "pluginConfig": [         {           "name": "loop",           "type": "bool",           "defaultValue": true         },         {           "name": "transTime",           "type": "int",           "defaultValue": 24,           "minValue": 1,           "maxValue": 600         },         {           "name": "delayTime",           "type": "int",           "defaultValue": 0,           "minValue": 0,           "maxValue": 600         },         {           "name": "linDirection",           "type": "string",           "defaultValue": "right",           "strings": [             "left",             "right",             "up",             "down"           ]         }       ]     },     {       "uuid": "23e70ff4-458c-4852-826f-9315d89ee6ed",       "name": "Meteor Shower",       "description": "Like a meteor shower of music notes,          watch as a stardust of colors sprinkle across the panels to the pitch of the music.          Colors on the left indicate low notes while the right represents higher notes.",       "author": "Nanoleaf",       "type": "rhythm",       "tags": [         "party",         " electronic",         " vibrant",         " pop"       ],       "features": []     }   ] }`

### 3.2.5. Reserved Effect Names

The Effect name is used to give the user a snapshot of what is currently displayed. As such, we reserve the following names for indicating certain Panels modes:

|     |     |
| --- | --- |
| **Reserved Name** | **Description** |
| \*Static\* | Panels hold static colors defined by the user |
| \*Dynamic\* | Temporary Dynamic Effect defined by user |
| \*Solid\* | User-defined through Universal Color |

### 

### 3.2.6. Effect Types

### 

|     |     |
| --- | --- |
| **Effect Types** | **Description** |
| \*Custom\* | Creates any arbitrary effect using a condensed panel-resolved timeline message format |
| extControl | External Control (Streaming) opens a UDP socket connection over the local network to stream control data frames to controller/control square. |

### 

#### 3.2.6.1. Custom

Custom effect types are used for Static or arbitrary effects and use the "animData" property. The format is as follows. This allows developers to define ‚"frame-by-frame" Effects.  
 

Copy      numPanels; panelId0; numFrames0; RGBWT01; RGBWT02; ... RGBWT0n(0);
      panelId1; numFrames1; RGBWT11; RGBWT12; ... RGBWT1n(1); ... ... panelIdN;
      numFramesN; RGBWTN1; RGBWTN2; ... RGBWTNn(N);

  
Note that there are no newline characters in the string, and that essentially this is linearization of a 2D Matrix, with N panels, and a certain number of frames per panel. Each panel need not have the same number of frames. Each RGBWTij; collection is a RGBWT vector and is represented as follows:

CopyR; G; B; W; T;

Note that the W; element is currently ignored. The white LED is used automatically during white balancing by Panels based on its internal calibrations. To avoid lengthy initial transitions that may not look visually pleasing, we introduce the concept of a start frame. If T is set to -1, it is considered a start frame which is initialized instantaneously. For logical consistency, a start frame can ONLY occur for the first frame of every panel. Start frames are not included if the effect is set to loop, which is why T=-1 is used instead of T=0. An RGBWT collection directs a panel to move to the specified RGBW color in a transition time of T\*100ms (1T = 100ms). This approach, coupled with a single frame per panel, is how one creates Static Effects.  
  
Examples  
Temporary static display  
 

Copy`{   "command": "display/add",   "animType": "static",   "animData": "3 82 1 255 0 255 0 20 60 1 0 255 255 0 20 118 1 0 0 0 0 20",   "loop": false,   "palette": [{       "hue": 0,       "saturation": 100,       "brightness": 0   }, {       "hue": 30,       "saturation": 100,       "brightness": 10   }],   "colorType": "HSB" }`

Copy _= 3_
 _= 1_
**_Panel 1:_**
_PanelId: 82, R:255, G:0, B:255, W:0, TransitionTime:20\*0.1s_
**_Panel 2:_**
_PanelId: 60, R:0, G:255, B:255, W:0, TransitionTime:20\*0.1s_
**_Panel 3:_**
_PanelId:118, R:0, G:0, B:0, W:0, TransitionTime:20\*0.1s_

Note the use of the static effect type to display a static effect.

#### 3.2.6.2. External Control (extControl)

Allows control over a UDP socket connection to stream control data to stream control data to the Panels. A request is sent to the controller to activate stream control to the Panels. The External Control mode can only be activated in display mode. The write command containing animType as extControl (see Section 5.7) will result in a 200 OK response with the body containing the IP address, the port number, and the protocol type for streaming control (Response only for Light Panels only).  
Example response (Light Panels only):

Copy`{   "streamControlIpAddr": "192.168.2.231",   "streamControlPort": ,   "streamControlProtocol": "udp" }`

The frame format is very similar to the custom animation type with the exception that it is sent as an array of bytes, instead of a human readable string. The conversion is done by considering each element as a separate byte as demonstrated by the examples below. Note that each frame need not include all panels if there is no transition for that panel at that time.

We recommend a stream rate of no faster than 10Hz. You do not need to update every panel every frame. If you specify a transition time larger than 1 (equivalent to 100ms), the panel will transition over that period. If you update the panel again before it completes, it will start a new transition to the target color from its current intermediate state from the previous transition.

Note that the White LED element is currently ignored. The white LED is used automatically during white balancing by Panels based on its internal calibrations.

The External Controller must not stream data at a rate higher than 10Hz. (100ms transitions are handled at a panel level and as such, will be smooth even at a 10Hz update frame rate)

Light Panels and Canvas use a different external control streaming protocols. Light Panels uses stream control v1, while Canvas uses stream control v2.

An example for both is given below:

To set the following colors on Light Panels:

|     |     |     |
| --- | --- | --- |
| **PanelId** | **Color** | **Transition Time** |
| 123 | {255, 0, 0} | 9   |
| 223 | {0, 255, 0} | 24  |
| 67  | {0, 0, 255} | 32  |

The stream control (v1) message would look like

Copy3  --> nPanels
123 1 255 0 0 0 9  --> set panel color
223 1 0 255 0 0 24  --> set panel color
67 1 0 0 255 0 32 --> set panel color

Breakdown of Set Panel Color :

|     |     |     |
| --- | --- | --- |
| 123 | 255 0 255 0 | 9   |
| Panel Id | {R G B W} Color | Transition time |

In order to support the much larger number of panels on Canvas, the size of the nPanels, panelId and tranitionTime fields have been increased from 1B to 2B. The nFrames field has been dropped as it was set to 1 in v1 anyway

To set the following colors on Canvas

|     |     |     |
| --- | --- | --- |
| **PanelId** | **Color** | **Transition Time** |
| 374 | {255, 0, 255, 0} | 12  |
| 651 | {255, 255, 0, 0} | 128 |
| 235 | {0, 255, 255, 0} | 451 |

The stream control (v2) message would look like

Copy`0 3  ‚---> nPanels`
`1 118 255 0 255 0 0 12  ‚---> Set panel color`
`2 139 255 255 0 0 0 128  ‚---> Set panel color`
`0 235 0 255 255 0 1 195 ‚---> Set panel color`

Breakdown of Set Panel Color :

|     |     |     |
| --- | --- | --- |
| 1 118 | 255 0 255 0 | 0 12 |
| Panel Id (Big Endian) | {R G B W} Color | Transition time (Big Endian) |

\*Note that all the multi-byte fields are represented in Big Endian format

Since this is transmitted as a byte stream, the message would look like, with each byte represented by hexadecimal digits:

Copy_0x00 0x03 0x01 0x76 0xFF 0x00 0xFF 0x00 0x00 0xC 0x02 0x8B 0xFF 0xFF
0x00 0x00 0x00 0x80 0x00 0xEB 0x00 0xFF 0xFF 0x00 0x01 0xC3_

## 3.3. Layout Data

The following describes the data format used to communicate the user‚Äôs detected Panels layout:

|     |     |     |     |
| --- | --- | --- | --- |
| **Item** | **Description** |     |     |
| numPanels | the number of panels. For Light Panels this is EXCLUSIVE of the controller For Canvas this in INCLUSIVE of the control square |     |     |
| sideLength | The length of a panel side, in pixels, that is used in calculation of the centroid location. |     |     |
| panelId | Panel identifying number for the nth panel. |     |     |
| x   | X coordinate of the centroid of the nth panel |     |     |
| y   | Y coordinate of the centroid of the nth panel |     |     |
| o   | Orientation of the nth panel. |     |     |
| ShapeType | Name | Value | Side Length |
| Triangle | 0   | 150 |
| Rhythm | 1   | N/A |
| Square | 2   | 100 |
| Control Square Master | 3   | 100 |
| Control Square Passive | 4   | 100 |
| Hexagon (Shapes) | 7   | 67  |
| Triangle (Shapes) | 8   | 134 |
| Mini Triangle (Shapes) | 9   | 67  |
| Shapes Controller | 12  | N/A |
| Elements Hexagons | 14  | 134 |
| Elements Hexagons - Corner | 15  | 33.5 / 58 |
| Lines Connector | 16  | 11  |
| Light Lines | 17  | 154 |
| Light Lines - Single Zone | 18  | 77  |
| Controller Cap | 19  | 11  |
| Power Connector | 20  | 11  |
| Nanoleaf 4D Lightstrip | 29  | 50  |
| Skylight Panel | 30  | 180 |
| Skylight Controller Primary | 31  | 180 |
| Skylight Controller Passive Mode | 32  | 180 |

_Note: As of 5.0.0 the side length field is now deprecated since it cannot represent the side lengths of multiple shapes in order to support the connect+ feature. It will still exist in the JSON, but its value will be set to 0.  
The side length must now be inferred from the shapeType. It will not change for a given Shape and can be treated as a constant._

In the case of Light Panels where controller has no lighting function, it is not included in the data provided by the API. In the case of Canvas/Skylight where controller square acts as regular panel, it is included in the data provided by the API.

Each element in the LayoutData json array corresponds to a panel with the specified panelId and its location and orientation specified in 2D cartesian space. The (X, Y) pair corresponds to the position of the panel’s centroid and o corresponds to its orientation, which is measured counter clockwise. Along with the sideLength, the orientation and the coordinates, the panel vertices position in cartesian space can be calculated and the panel can be drawn in 2D space on a screen.

Note that the Global Orientation of the system is typically provided by the user through the Nanoleaf App. This defines the canvas rotation, and normally represents the user‚Äôs preference for viewing the panel layout. The panel layout data will always be the same, regardless of the Global Orientation, so it is the responsibility of the developer to account for this if they wish to draw the layout on a screen.

### 3.3.1. Layout Examples

#### 3.3.1.1. Light Panels ( Ten Panels )

![Ten Panel LP](/assets/openapi-images/ten-panel-LP.png)

Sample layout from the Central Controller:

Copy`{   "numPanels": 10,   "sideLength": 150,   "positionLayout": [     {       "panelId": 1,       "x": 100,       "y": 100,       "o": 60,       "shapeType": 0     },     {       "panelId": 2,       "x": 324,       "y": 56,       "o": 0,       "shapeType": 0     },     {       "panelId": 3,       "x": 249,       "y": -159,       "o": 60,       "shapeType": 0     },     {       "panelId": 4,       "x": 174,       "y": 56,       "o": 240,       "shapeType": 0     },     {       "panelId": 5,       "x": 324,       "y": -29,       "o": 60,       "shapeType": 0     },     {       "panelId": 6,       "x": -49,       "y": 100,       "o": 60,       "shapeType": 0     },     {       "panelId": 7,       "x": 399,       "y": 99,       "o": 300,       "shapeType": 0     },     {       "panelId": 8,       "x": 174,       "y": -29,       "o": 60,       "shapeType": 0     },     {       "panelId": 9,       "x": 25,       "y": 56,       "o": 120,       "shapeType": 0     },     {       "panelId": 10,       "x": 249,       "y": -73,       "o": 240,       "shapeType": 0     }   ] }`

#### 3.3.1.2. Light Panels ( Six Panels )

![Six Panel LP](/assets/openapi-images/six-panel-LP.png)

Sample layout from the Central Controller:

Copy`{   "numPanels": 10,   "sideLength": 150,   "positionLayout": [     {       "panelId": 1,       "x": 100,       "y": 100,       "o": 0,       "shapeType": 0     },     {       "panelId": 2,       "x": 25,       "y": -29,       "o": 240,       "shapeType": 0     },     {       "panelId": 3,       "x": 174,       "y": -29,       "o": 120,       "shapeType": 0     },     {       "panelId": 4,       "x": 99,       "y": 13,       "o": 180,       "shapeType": 0     },     {       "panelId": 5,       "x": 99,       "y": -159,       "o": 0,       "shapeType": 0     },     {       "panelId": 6,       "x": 25,       "y": -116,       "o": 60,       "shapeType": 0     }   ] }`

#### 3.3.1.3. Canvas ( Six Panels )

![Six Panel Canvas](/assets/openapi-images/six-panel-Canvas.png)

Sample layout from the Control Square:

Copy`{   "numPanels": 6,   "sideLength": 100,   "positionData": [     {       "panelId": 46095,       "x": 250,       "y": 100,       "o": 0,       "shapeType": 2     },     {       "panelId": 9489,       "x": 150,       "y": 100,       "o": 0,       "shapeType": 2     },     {       "panelId": 22817,       "x": 50,       "y": 100,       "o": 0,       "shapeType": 2     },     {       "panelId": 31391,       "x": 0,       "y": 0,       "o": 180,       "shapeType": 2     },     {       "panelId": 26038,       "x": 50,       "y": 200,       "o": 0,       "shapeType": 2     },     {       "panelId": 19479,       "x": 150,       "y": 250,       "o": 180,       "shapeType": 2     }   ] }`

## 3.4. Brightness

Brightness is a central component of any smart lighting system, and Nanoleaf Panels are no exception. It‚Äôs worth differentiating two concepts:

*   Universal Brightness
*   Palette Color Brightness

The Universal Brightness is what a user normally controls. This works the same way any smart light works. If things are too bright, dim the (universal) brightness and vice versa.

You will notice that each Palette color also has a "brightness". This is because Palette colors are specified in HSB, which is analogous to HSV. Note that there is a good deal of subtlety around this difference because screen colours fade to black (V=0), whereas Panels colours fade to white (or whatever ambient lighting is around, B=0).

Panels' Universal Brightness fades Panels regardless of mode (Static, Dynamic, Solid). You can consider it like a universal multiplier. If you have palette colors with varying brightnesses, their relative ratios will be maintained by Panels regardless of universal brightness .

## 3.5. Events

Developers can register and listen to events from Nanoleaf Devices using Server-Sent Events (SSE).

*   Light Panels requires version > 3.1.0
*   Canvas requires version > 1.10

Please note, only Canvas supports Touch events at this time.

### 3.5.1. Registering and Receiving Events

To register for events, a client should send a GET request to the /events endpoint with a query string containing a comma separated list of the event types it wishes to register for.

For example if a client wished to register for the state (id=1), layout (id=2) and touch (id=3) events, it would sent the following request:

Example request:

Copy_GET /api/v1//events?id=1,2,4 HTTP/1.1_

CopyHost: 192.168.0.2:16021

The Panels will respond and keep the connection open and keep transmitting events as they take place on the controller

Sample response:

When an event occurs, the eventId is sent with the id field followed by a newline character, with the actual event json being sent as part of the data field.

Copy_HTTP/1.1 200 OK_
_Content-Type: text/event-stream_
_._
_._
_id:_ 
_data:_ 

_._
_._
_id:_ 
_data:_ 

### 3.5.2. Events Types

There are currently 4 different event types available (with more types to be added in the future)

|     |     |
| --- | --- |
| **Event Type** | **Event Type Id** |
| **State:** | 1   |
| **Layout** | 2   |
| **Effects** | 3   |
| **Touch:** | 4   |

#### 3.5.2.1. State

The state event (id : 1) is sent out whenever the state of the Canvas changes. It covers changes on attributes from the /state endpoint. Examples include power, brightness, e.t.c

|     |     |
| --- | --- |
| **Attribute** | **Attribute ID** |
| on  | 1   |
| brightness | 2   |
| hue | 3   |
| saturation | 4   |
| cct | 5   |
| colorMode | 6   |

Example event json:

Copy`{   "events": [     {       "attr": 1,       "value": true     },     {       "attr": 2,       "value": 40     },     {       "attr": 3,       "value": 72     },     {       "attr": 4,       "value": 2700     }   ] }`

#### 3.5.2.2. Layout

The layout event (id : 2) is sent out whenever there are changes to attributes covered by the /layout endpoint.

|     |     |
| --- | --- |
| **Attribute** | **Attribute ID** |
| layout | 1   |
| globalOrientation | 2   |

Example event json:

Copy`{   "events": [     {       "attr": 1,       "value": {         "numPanels": 3,         "sideLength": 100,         "positionData": [           {             "panelId": 37317,             "x": 200,             "y": 200,             "o": 0,             "shapeType": 2           },           {             "panelId": 1038,             "x": 200,             "y": 100,             "o": 90,             "shapeType": 2           },           {             "panelId": 7930,             "x": 200,             "y": 0,             "o": 90,             "shapeType": 2           }         ]       }     },     {       "attr": 2,       "value": 90     }   ] }`

#### 3.5.2.3. Effects

The effects (id: 3) event is sent out whenever the attributes covered under the /effects endpoint changes Example Event Json:

Copy`{   "events": [     {       "attr": 1,       "value": "Flames"     }   ] }`

#### 3.5.2.4. Touch

Touch (id: 4) events are fired whenever the control square detects touch interaction with the layout.

The event json, carries an array of objects, each indicating the last detected gesture, and the panelId that the gesture was detected on (if applicable). The panelId field carries a meaningful value only if the gesture can be associated with a specific panel such as for double tap and single tap gestures. For swipe gestures, the panelId is set to -1.

|     |     |     |
| --- | --- | --- |
| **Gesture** | **Gesture ID** | **Can be associated to a panel** |
| Single Tap | 0   | Yes |
| Double Tap | 1   | Yes |
| Swipe Up | 2   | No  |
| Swipe Down | 3   | No  |
| Swipe Left | 4   | No  |
| Swipe Right | 5   | No  |

Example Event Json:

Copy`{   "events": [     {       "gesture": 0,       "panelId": 7     },     {       "gesture": 1,       "panelId": 7     },     {       "gesture": 3,       "panelId": -1     }   ] }`

##### 3.5.2.4.1. Gesture Ids

|     |     |     |
| --- | --- | --- |
| **Gesture** | **Gesture ID** | **Can be associated to a panel** |
| Single Tap | 0   | Yes |
| Double Tap | 1   | Yes |
| Swipe Up | 2   | No  |
| Swipe Down | 3   | No  |
| Swipe Left | 4   | No  |
| Swipe Right | 5   | No  |

##### 3.5.2.4.2. Touch Stream Data

As of firmware version 1.4.0+, clients will be able to optionally register for fine resolution, low latency, touch data stream from the canvas controller over a UDP socket, if the client has also registered for Touch Events.

The Touch Data Stream is active for as long as the SSE session is active. Once the client closes the SSE session, the Touch Data Stream will be disabled as well.

##### 3.5.2.4.3. Enabling Touch Stream Data

In order to ensure low latency, just like in StreamControl, the Touch Data is streamed as binary data over a UDP socket.

The controller will stream the data to the ip address of the client from whom the event request was received and a port as specified by the client.

The port can be specified through a custom HTTP header in the HTTP GET request for touch events. The custom header is as shown below:

CopyTouchEventsPort: 

##### 3.5.2.4.4. Touch Data Stream Format

Once the GET request is received by the controller, it will also start streaming Touch Data to the UDP Socket on the specified port. The format is as follows:

Copy  _<(touchType = 3bits |
touchStrength = 4bits)1 = 1B>_ 

 _<(touchType = 3bits | touchStrength = 4bits)2 =
1B>_ 

 _<(touchType = 3bits | touchStrength = 4bits)3 =
1B>_ 

 _<(touchType = 3bits | touchStrength = 4bits)4 =
1B>_ 
_._
_._
_._
 _<(touchType = 3bits | touchStrength = 4bits)\_n
= 1B>_ 

All multi byte fields are Big Endian. The first 2 Bytes represents the number of panels that are currently undergoing a touch event. After that follows nPanels number of 5B packets, each packet representing touch data for that panel.

The first 2B of the packet represents the panelId of the panel that is currently undergoing a touch event. The next 1 byte is a combination of the Touch Type and Touch Strength:

**Touch Type and Touch Strength Byte**

|     |     |     |     |     |     |     |     |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Reserved | Touch Type | Touch Strength |
| Bit 7 | Bit 6 | Bit 5 | Bit 4 | Bit 3 | Bit 2 | Bit 1 | Bit 0 |
|     |     |     |     |     |     |     |     |

The last 2 bytes represent the panelId of the panel swiped from only in case of a swipe event. This value is otherwise set to 0xFF

Touch Types are defined as follows:

|     |     |
| --- | --- |
| **Value** | **Description** |
| 0   | Hover: The user is hovering their hand over a panel |
| 1   | Down: The user has placed their hand down on a panel |
| 2   | Hold: The user has held their hand down on a panel |
| 3   | Up: the user has removed their hand from a panel |
| 4   | Swipe: The user has swiped from one panel to another. The panelId of the panel swiped from is sent along as well |

##### 3.5.2.4.5. Touch Life Cycle

Touch life cycle describes the typical sequence of touch events that occur when a user would touch a panel. This is a guide only, and not all touch events are necessarily visited.

A typical touch life cycle will look something like this:

![Touch Life Cycle](/assets/openapi-images/touch_lifecycle.png)

## 3.7. Discovery

The Panels use standard technologies for local network discovery.

### 3.6.1. mDNS ( Bonjour )

Panels can be discovered on the local network using mDNS under the service type: _\_nanoleafapi.\_tcp_

The service includes the IP and port (IPv4 and IPv6). Note that the port defaults to 16021, although we do not recommend hard-coding this information in your code.

The TXT record format is as follows:

|     |     |
| --- | --- |
| **Key** | **Description** |
| "md" | Model name of the product (i.e. NL22 / NL29/ NL42/ NL52/NL59/ NL64/ NL69) |
| "srcvers" | Firmware version of the product (i.e. 1.5) |
| "id" | Device ID. A randomly generated device ID that will change if a user resets the device (resets will remove API authorizations). |

### 3.6.2. SSDP

Panels can be discovered on the local network using SSDP (Simple Service Discovery Protocol/1.0).

The multicast address for SSDP multicast messages is (per spec):

Copy _239.255.255.250:1900_

The UUID of each device is randomly generated based on RFC 4122 (A Universally Unique IDentifier (UUID) URN Namespace) Variant 4 (Section 4.4).

#### 3.6.2.1. SSDP NOTIFY

Panels will multicast SSDP notify messages on the multicast address at a regular interval (default: 1 minute). A notify message contains the HTTP header: NOTIFY \* HTTP/1.1. Contents of the remaining headers in a notify message are:

|     |     |
| --- | --- |
| **Header** | **Header body** |
| "Host" | Multicast IP address: 239.255.255.250:1900 |
| "NT" | Service type: Nanoleaf\_aurora:light / nanoleaf:nl29 |
| "NTS" | ssdp:alive |
| "USN" | UUID of the Panels. For example: uuid:123456-1234-1234-123456789abc |
| "Location" | Local IP address used by OpenAPI. For example: http://192.168.1.140:16021 |
| "Cache-Control" | Notify interval. For example: max-age = 60 |
| "nl-deviceid" | Device ID. Updates after a Panels are reset. For example: 46:EC:1B:0D:67:32 |
| "nl-devicename" | Device Name. Includes a device identifier based on MAC address. Could be updated by an iOS user using WAC. |

OpenAPI can be accessed by using the IP address given by Location. Note that the port number is 16021 by default although we recommend getting this information dynamically from SSDP.

No notify message will be multicast if the Panels are unavailable.

#### 3.6.2.2. SSDP M-SEARCH

Panels also listens to the multicast address for SSDP m-search messages. An m-search message intended for discovery using Panels OpenAPI must contain in its ST header the service type ssdp:all or nanoleaf\_aurora:light.

Panels will respond by unicast to an m-search message containing one of the above service types with a response, containing the HTTP header: HTTP/1.1 200 OK. The remaining headers in a response are:

|     |     |
| --- | --- |
| **Header** | **Header body** |
| "S" | UUID of the Device. For example: uuid:123456-1234-1234-123456789abc |
| "Ext" | (empty) |
| "Cache-Control" | Notify interval. For example: no-cache="Ext", max-age = 60 |
| "ST" | Service type: Nanoleaf\_aurora:light / nanoleaf:nl29 |
| "USN" | UUID of the Device. For example: uuid:123456-1234-1234-123456789abc |
| "Location" | Local IP address used by OpenAPI. For example: http://192.168.1.140:16021 |
| "nl-deviceid" | Device ID. Updates after Panels are reset. For example: |

OpenAPI can be accessed by using the IP address given by Location. Note that the port number is 16021 by default although we recommend getting this information dynamically from SSDP.

If the m-search message provides a non-zero value in its MX or MAX\_COUNT header (denoted by M), Panels will send a response after a delay of d seconds, where d is chosen uniformly at random from the set of integers {0, 1, ..., M}.

No response will be sent if the Panels are unavailable.

#### 3.6.2.3. SSDP BYEBYE

If the Panels become unavailable, an SSDP byebye message is multicast on the multicast address with the HTTP header: NOTIFY \* HTTP/1.1. Contents of the remaining headers in a byebye message are:

|     |     |
| --- | --- |
| Header | Header body |
| "Host" | Multicast IP address: 239.255.255.250:1900 |
| "NT" | Service type: Nanoleaf\_aurora:light / nanoleaf:nl29 |
| "NTS" | ssdp:byebye |
| "USN" | UUID of the Device. For example: uuid:123456-1234-1234-123456789abc |

# 4\. API JSON Structure

The following is an example of the internal JSON structure used to organize Panels' device data used by the Open API. It is only accessed by Open API calls, although its relevant contents may be modified by any user-accessible control method, whether it be the Nanoleaf App, HomeKit, Alexa, IFTTT, or other future control path.

## 4.1. Light Panels

Copy`{   "name": "Light Panels Name",   "serialNo": "S16331A0217",   "manufacturer": "Nanoleaf",   "firmwareVersion": "1.5.0",   "model": "NL22",   "state": {     "on": {       "value": false     },     "brightness": {       "value": 100,       "max": 100,       "min": 0     },     "hue": {       "value": 0,       "max": 360,       "min": 0     },     "sat": {       "value": 0,       "max": 100,       "min": 0     },     "ct": {       "value": 4000,       "max": 100,       "min": 0     },     "colorMode": "effect"   },   "effects": {     "select": "Flames",     "effectsList": [       "Color Burst",       "Flames",       "Forest",       "Inner Peace",       "Nemo",       "Northern Lights",       "Romantic",       "Snowfall"     ]   },   "panelLayout": {     "layout": {       "numPanels": 2,       "sideLength": 150,       "positionData": [         {           "panelId": 107,           "x": -74,           "y": 43,           "o": 180,           "shapeType": 0         },         {           "panelId": 114,           "x": -149,           "y": 0,           "o": 360,           "shapeType": 0         }       ]     },     "globalOrientation": {       "value": 120,       "max": 360,       "min": 0     }   },   "rhythm": {     "rhythmConnected": true,     "rhythmActive": true,     "rhythmId": 309,     "hardwareVersion": "1.4",     "firmwareVersion": "1.7-R",     "auxAvailable": true,     "rhythmMode": 1,     "rhythmPos": {       "x": 299,       "y": -86,       "o": 300     }   } }`

## 4.2. Canvas

Copy`{   "name": "Canvas Name",   "serialNo": "S18374A0015",   "manufacturer": "Nanoleaf",   "firmwareVersion": "1.1.0",   "model": "NL29",   "discovery": {},   "effects": {     "effectsList": [       "Color Burst",       "Falling Whites",       "Fireworks",       "Flames",       "Forest",       "Inner Peace",       "Meteor Shower",       "Nemo",       "Northern Lights",       "Paint Splatter",       "Pulse Pop Beats",       "Radial Sound Bar",       "Rhythmic Northern Lights",       "Romantic",       "Sound Bar",       "Streaking Notes"     ],     "select": "Northern Lights"   },   "panelLayout": {     "globalOrientation": {       "value": 0,       "max": 360,       "min": 0     },     "layout": {       "numPanels": 12,       "sideLength": 100,       "positionData": [         {           "panelId": 46095,           "x": 400,           "y": 100,           "o": 0,           "shapeType": 2         },         {           "panelId": 31391,           "x": 300,           "y": 100,           "o": 270,           "shapeType": 2         },         {           "panelId": 39497,           "x": 300,           "y": 200,           "o": 270,           "shapeType": 2         },         {           "panelId": 42722,           "x": 200,           "y": 200,           "o": 270,           "shapeType": 2         },         {           "panelId": 26430,           "x": 200,           "y": 100,           "o": 270,           "shapeType": 2         },         {           "panelId": 18778,           "x": 100,           "y": 250,           "o": 90,           "shapeType": 2         },         {           "panelId": 35118,           "x": 100,           "y": 150,           "o": 0,           "shapeType": 2         },         {           "panelId": 63476,           "x": 0,           "y": 200,           "o": 270,           "shapeType": 2         },         {           "panelId": 13987,           "x": 0,           "y": 100,           "o": 270,           "shapeType": 2         },         {           "panelId": 24430,           "x": 400,           "y": 200,           "o": 0,           "shapeType": 2         },         {           "panelId": 24564,           "x": 350,           "y": 300,           "o": 180,           "shapeType": 2         },         {           "panelId": 8223,           "x": 250,           "y": 300,           "o": 90,           "shapeType": 2         }       ]     }   },   "state": {     "brightness": {       "value": 100,       "max": 100,       "min": 0     },     "colorMode": "effect",     "ct": {       "value": 3000,       "max": 6500,       "min": 1200     },     "hue": {       "value": 0,       "max": 360,       "min": 0     },     "on": {       "value": true     },     "sat": {       "value": 0,       "max": 100,       "min": 0     }   } }`

# 5\. API Reference

API endpoint follows the hierarchical structure of the internal JSON.

## 5.1. Authorization

A user is authorized to access the OpenAPI if they can demonstrate physical access of Panels. This is achieved by: Holding the on-off button down for 5-7 seconds until the LED starts flashing in a pattern.

Please note that Nanoleaf Skylight does not have physical on-off button, so the OpneAPI authorization window has to open through the Nanoleaf app. On the Nanoleaf mobile app, you can open that window by going to the device settings of your paired Nanoleaf Skylight and clicking on "Connect to API".

Sending a POST request to the authorization endpoint /api/v1/new within 30 seconds of activating pairing. The POST response will contain a randomly generated authorization token:

Copy  _{"auth\_token" : "xxxxxxxxxxxxxxxxxxxxx ... xxx"}_

In the following, the authorization token string is denoted by

### 5.1.1. Add User

|     |     |
| --- | --- |
| Endpoint | /api/v1/new |
| Request | POST |
| Request Body | None |
| Response | 200 OK |
| Response Body | {"auth\_token" : } |
| Error | 401 Unauthorized 403 Forbidden 422 Unprocessable Entity |

### 5.1.2. Delete User

|     |     |
| --- | --- |
| Endpoint | /api/v1/ |
| Request | DELETE |
| Request Body | None |
| Response | 204 No Content |
| Response Body | None |
| Error | 401 Unauthorized 500 Internal Server Error |

## 5.2. Get All Panel Info

|     |     |
| --- | --- |
| Endpoint | /api/v1// |
| Request | GET |
| Request Body | None |
| Response | 200 OK |
| Response Body | See section 4. API JSON Structure |
| Error | 401 Unauthorized 422 Unprocessable Entity |

## 5.3. State

When writing state information, you set the value of a state or operate using increment where applicable. Increment can be positive or negative and will only go to the min or max value.

### 5.3.1. On/Off

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/on_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : true}_ _OR_ _{value : false}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{on : {value:true}}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400 Bad Request_ _404 Resource Not Found_ |

### 5.3.2. Brightness

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/brightness_ |
| _Request_ | _GET_ |
| _Request Body_ | _none_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : 100, max : 100, min : 0}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{brightness : {value:100, duration:30}}_ _OR_ _{brightness : {increment:-10}}_ _Duration field is optional, and specified in seconds_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _none_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ _422-Unprocessible Entity_ |

### 5.3.3. Hue

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/hue_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : 100, max : 360, min : 0}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{hue : {value:120}}_ _OR_ _{hue : {increment:20}}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ _422-Unprocessible Entity_ |

### 5.3.4. Saturation

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/sat_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : 100, max : 100, min : 0}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{sat : {value: 20}}_ _OR_ _{sat : {increment: 10}}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ _422-Unprocessible Entity_ |

### 5.3.5. Color Temperature

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/ct_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : 4000, max : 6500, min : 1200}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{ct : {value: 3000}}_ _OR_ _{ct : {increment: -100}}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ _422-Unprocessible Entity_ |

### 5.3.6. Color Mode

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/state/colorMode_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _ct_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

## 5.4. Effects

### 5.4.1. Select

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/effects/select_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _\*Solid\*_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/effects_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{select : Snowfall}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ |

### 5.4.2. List

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/effects/effectsList_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _\[Color Burst,Flames,Forest,Inner Peace,Nemo,Northern Lights,Romantic,Snowfall\]_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.4.3. Write

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/effects_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{write : {command : request, animName : Northern Lights}}_ |
| _Response_ | _200 OK, 204 No Content_ |
| _Response Body_ | _{animName:Northern Lights,loop:true,palette:\[{hue:227,saturation:100,brightness:99},{hue:182,saturation:100,brightness:100},{hue:125,saturation:100,brightness:93},{hue:62,saturation:100,brightness:100},{hue:31,saturation:100,brightness:100},{hue:2,saturation:100,brightness:100},{hue:307,saturation:100,brightness:100}\],transTime:{maxValue:20,minValue:20},windowSize:2,flowFactor:0,delayTime:{maxValue:20,minValue:5},colorType:HSB,animType:wheel,explodeFactor:0,brightnessRange:{maxValue:0,minValue:0},direction:left}_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ |

## 5.5. Panel Layout

### 5.5.1. Global Orientation

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/panelLayout/globalOrientation_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{value : 0, max : 360, min : 0}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/panelLayout_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{globalOrientation : {value: 120}}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _400-Bad request_ _404-Resource Not Found_ _422-Unprocessible Entity_ |

### 5.5.2. Layout

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/panelLayout/layout_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{numPanels:2,sideLength:150,positionData:\[{panelId:107,x:-74,y:43,o:180},{panelId:114,x:-149,y:0,o:360}\]}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

## 5.6. Identify

Causes the panels to flash in unison. This is typically used to help users differentiate between multiple panels.

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/identify_ |
| _Request_ | _PUT_ |
| _Request Body_ | _None_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _401-Unauthorized_ _404-Resource Not Found_ |

## 5.7. External Control ( Streaming )

Allows control over a UDP socket connection to stream control data to stream control data to the Panels. A request can be made as shown in the table below activate stream control to the Panels. Please note the Light panels support streaming control version v1 and v2. Canvas supports stream control version v2 and requires specifying the version in the request. If the extControlVersion key is omitted, v1 is assumed, which would return an error for Canvas.

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/effects_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{_ _command: display,_ _animType: extControl,_ _extControlVersion: v2_ _}_ |
| _Response_ | _204 OK_ |
| _Response Body_ | _Canvas:_ _No response body._ _address: same as the IP address of the controller_ _port: 60222_ _protocol: udp_ _Light Panels:_ _{_ _streamControlIpAddr: 192.168.2.231,_ _streamControlPort: port;,_ _streamControlProtocol: udp_ _}_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

## 5.8. Rhythm ( Light Panels Only )

The /rhythm Endpoint allows you to get information about the Rhythm and switch the Rhythm mode from microphone to aux input.

### 5.8.1. Rhythm Connected

Indicates if the Rhythm is connected to the Light Panels or not. Note in the case of Canvas, the rhythm is built into the control square

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmConnected_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _true_ _OR_ _false_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.2. Rhythm Active

Indicates if the Rhythm's microphone is currently active or not.

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmActive_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _true_ _OR_ _false_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.3. Rhythm Id

Indicates the Rhythm's Id in the Panels system

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmId_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _415_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.4. Rhythm Hardware Version

Indicates the Rhythm's hardware version

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/hardwareVersion_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _"1.4"_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.5. Rhythm Firmware Version

Indicates the Rhythm's firmware version

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/firmwareVersion_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _"1.8"_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.6. Rhythm Aux Available

Indicates if an aux cable (3.5mm) is currently connected to the Rhythm

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/auxAvailable_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _true_ _OR_ _false_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.7. Rhythm Mode

Allows the user to control the sound source for the Rhythm. Writing a 0 to this field sets the Rhythm's sound source to the microphone, and writing a 1 to the field sets the sound source to the aux cable

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmMode_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _0 (Microphone)_ _OR_ _1 (Aux Cable)_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmMode_ |
| _Request_ | _PUT_ |
| _Request Body_ | _{rhythmMode: 1}_ |
| _Response_ | _204 No Content_ |
| _Response Body_ | _None_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.8.8. Rhythm Position

Indicates the position and orientation of the Rhythm in Panels' layout

|     |     |
| --- | --- |
| _Endpoint_ | _/api/v1/auth\_token/rhythm/rhythmPos_ |
| _Request_ | _GET_ |
| _Request Body_ | _None_ |
| _Response_ | _200 OK_ |
| _Response Body_ | _{ x: -149, y: -86, o: 300 }_ |
| _Error_ | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

## 5.9 Screen Mirror

### 5.9.1 Activate Screen Mirror

|     |     |
| --- | --- |
| Endpoint | /api/v1/auth\_token/effects |
| Request | PUT |
| Request Body | {"command": "activateScreenMirror", "screenMirrorMode": }<br><br>*   1D - int 6<br>*   2D - int 2<br>*   3D - int 3<br>*   4D - int 5 |
| Response | _200 OK_ |
| Response Body |     |
| Error | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.9.2 Activate Music Sync

|     |     |
| --- | --- |
| Endpoint | /api/v1/auth\_token/effects |
| Request | PUT |
| Request Body | { "command":"setScreenMirrorRhythmMode" "screenMirrorRhythmMode": }<br><br>{ "command":"setScreenMirrorRhythmModeType" "screenMirrorRhythmModeType": }<br><br>*   Pulse - int 0<br>*   Fireworks - int 1 |
| Response | _200 OK_ |
| Response Body |     |
| Error | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.9.3 Get Screen Mirror

|     |     |
| --- | --- |
| Endpoint | /api/v1/auth\_token/effects |
| Request | PUT |
| Request Body | { "command":"getScreenMirrorMode" } |
| Response | 200 OK |
| Response Body | { "screenMirrorMode": } |
| Error | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

### 5.9.4 Get Music Sync

|     |     |
| --- | --- |
| Endpoint | /api/v1/auth\_token/effects |
| Request | PUT |
| Request | { "command":"getScreenMirrorRhythmMode" } |
| Response | 200 OK |
| Response Body | { "screenMirrorRhythmMode": } |
| Error | _400-Bad Request_ _401-Unauthorized_ _404- Resource Not Found_ |

# 6\. Known Issues

1.5.0: - Min and max values returned for ct are wrong. They should be 1200-6500, but are returned 0-100.