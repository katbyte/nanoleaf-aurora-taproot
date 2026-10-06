<!-- Saved from https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/2298052616, version 27 of 2025-04-15. The .html beside this is the original. -->

# Motion Appendix

Racers | Streaking Notes | Sound Bar | Bounce | Highlight | Melt | Pulse | Wheel | Intertwine | Fade | Roller Coaster | Burst | Random | Organic | Flow

## Racers

|  |  |
|---|---|
| Name | Racers |
| UUID | `29929fc2-6905-41c0-a7d7-1582a8c11f17` |
| Description | Colors race each other at different speeds from one end to the other. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "defaultValue": 450,<br>      "minValue": 1,<br>      "maxValue": 600<br>    }<br>  ]<br>}</pre> |

## Streaking Notes

|  |  |
|---|---|
| Name | Streaking Notes |
| UUID | `337784c0-6c98-447a-8a9f-fc6ac44a8024` |
| Description | Each color of your palette is assigned from low notes to high notes. When that note plays, you'll see that color shoot across your panels |
| Developer | Nanoleaf |
| Type | rhythm |
| Plugin Config | <pre>N/A</pre> |

## Sound Bar

| Name | Wheel |
|---|---|
| UUID | `fe0d0ba8-741f-4210-940c-454eceed010frhythm` |
| Description | Two of your favorite colors compete in a dance-off to the intensity of music. The length of your setup will determine the direction the soundbar will go. Works best for softer music. |
| Developer | Nanoleaf |
| Type | rhythm |
| Plugin Config | <pre>N/A</pre> |

## Bounce

|  |  |
|---|---|
| Motion Name | Bounce |
| Motion UUID | `db8ce916-88de-484f-bdbb-afd732053105` |
| Motion Description | Colors overlap then bounce back and forth. |
| Motion Type | Color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "blockSize",<br>      "type": "int",<br>      "defaultValue": 5,<br>      "minValue": 3,<br>      "maxValue": 50<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "defaultValue": 200,<br>      "minValue": 1,<br>      "maxValue": 600<br>    }<br>  ]<br>}</pre> |

## Highlight

|  |  |
|---|---|
| Name | Highlight |
| UUID | `70b7c636-6bf8-491f-89c1-f4103508d642` |
| Description | Just like random, but you will mostly see the first color of your palette. The other colors fade in periodically. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 15<br>    },<br>    {<br>      "name": "mainColorProb",<br>      "type": "double",<br>      "minValue": 0,<br>      "maxValue": 100,<br>      "defaultValue": 80<br>    }<br>  ]<br>}</pre> |

## Melt

|  |  |
|---|---|
| Name | Melt |
| UUID | `30a017d4-e6a5-4647-8c41-5a7cd38ff907` |
| Description | Softly fading in and out, colors calmly cycle through with a blur effect - as if each shade is gradually melting into the next. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    }<br>  ]<br>}</pre> |

## Pulse

|  |  |
|---|---|
| Name | Pulse |
| UUID | `0794a6b3-f6cc-452d-a900-30081604b7ec` |
| Description | The perfect dreamy animation to help you slow down. Colors pulse in and out like a hushed heartbeat for the ultimate soothing effect. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    }<br>  ]<br>}</pre> |

## Wheel

|  |  |
|---|---|
| Name | Wheel |
| UUID | `6970681a-20b5-4c5e-8813-bdaebc4ee4fa` |
| Description | Provides a continuous moving gradient of color created from your palette. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "nColorsPerFrame",<br>      "type": "int",<br>      "minValue": 2,<br>      "maxValue": 50,<br>      "defaultValue": 2<br>    },<br>    {<br>      "name": "linDirection",<br>      "type": "string",<br>      "strings": [<br>        "left",<br>        "right",<br>        "up",<br>        "down"<br>      ],<br>      "defaultValue": "right"<br>    }<br>  ]<br>}</pre> |

## Intertwine

|  |  |
|---|---|
| Name | Intertwine |
| UUID | `5c4d343f-8eca-47e9-8c01-92662dff4eb6` |
| Description | Colors appear from both sides, weaving together as they overlap. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "blockSize",<br>      "type": "int",<br>      "defaultValue": 1,<br>      "minValue": 1,<br>      "maxValue": 20<br>    },<br>    {<br>      "name": "fadeOut",<br>      "type": "int",<br>      "defaultValue": 1,<br>      "minValue": 0,<br>      "maxValue": 20<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "defaultValue": 1,<br>      "minValue": 0,<br>      "maxValue": 600<br>    }<br>  ]<br>}</pre> |

## Fade

|  |  |
|---|---|
| Name | Fade |
| UUID | `b3fd723a-aae8-4c99-bf2b-087159e0ef53` |
| Description | The Light Panels cycle through your palette colors all together. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    }<br>  ]<br>}</pre> |

## Roller Coaster

|  |  |
|---|---|
| Name | Roller Coaster |
| UUID | `e2eb1818-07ac-495f-a10d-7c192d7df705` |
| Description | Keep your hands inside the vehicle at all times as you watch the colors go. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 500<br>    },<br>    {<br>      "name": "enableBackgroundColor",<br>      "type": "bool",<br>      "defaultValue": false<br>    }<br>  ]<br>}</pre> |

## Burst

|  |  |
|---|---|
| Name | Burst |
| UUID | `713518c1-d560-47db-8991-de780af71d1e` |
| Description | Your palette colors radiate out from the center of the panels. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    }<br>  ]<br>}</pre> |

## Random

|  |  |
|---|---|
| Name | Random |
| UUID | `ba632d3e-9c2b-4413-a965-510c839b3f71` |
| Description | This will take your palette colors and randomly animate them across your panels. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    }<br>  ]<br>}</pre> |

## Organic

|  |  |
|---|---|
| Name | Organic |
| UUID | `1dab05d3-07bf-4648-9d48-db8dd196ed28` |
| Description | Soft clusters of light appear, float, and dissolve in random yet natural feeling forms |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "scale",<br>      "type": "double",<br>      "minValue": 0.01,<br>      "maxValue": 5,<br>      "defaultValue": 1<br>    },<br>    {<br>      "name": "linDirection",<br>      "type": "string",<br>      "strings": [<br>        "left",<br>        "right",<br>        "up",<br>        "down",<br>        "none"<br>      ],<br>      "defaultValue": "none"<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "double",<br>      "defaultValue": 24,<br>      "maxValue": 600,<br>      "minValue": 1<br>    },<br>    {<br>      "name": "evolutionSpeed",<br>      "type": "double",<br>      "minValue": 0.01,<br>      "maxValue": 5,<br>      "defaultValue": 1<br>    },<br>    {<br>      "name": "stretch",<br>      "type": "double",<br>      "minValue": 0,<br>      "maxValue": 10,<br>      "defaultValue": 5<br>    }<br>  ]<br>}</pre> |

## Flow

|  |  |
|---|---|
| Name | Flow |
| UUID | `027842e4-e1d6-4a4c-a731-be74a1ebd4cf` |
| Description | Imagine you dump each color onto your panels. Watch the color flow in a direction of your choosing. |
| Developer | Nanoleaf |
| Type | color |
| Plugin Config | <pre>{<br>  "options": [<br>    {<br>      "name": "loop",<br>      "type": "bool",<br>      "defaultValue": true<br>    },<br>    {<br>      "name": "transTime",<br>      "type": "int",<br>      "minValue": 1,<br>      "maxValue": 600,<br>      "defaultValue": 24<br>    },<br>    {<br>      "name": "delayTime",<br>      "type": "int",<br>      "minValue": 0,<br>      "maxValue": 600,<br>      "defaultValue": 0<br>    },<br>    {<br>      "name": "linDirection",<br>      "type": "string",<br>      "strings": [<br>        "left",<br>        "right",<br>        "up",<br>        "down"<br>      ],<br>      "defaultValue": "right"<br>    }<br>  ]<br>}</pre> |
