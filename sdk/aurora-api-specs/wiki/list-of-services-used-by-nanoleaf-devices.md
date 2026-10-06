<!-- Saved from https://nanoleaf.atlassian.net/wiki/spaces/nlapid/pages/3410591752, version 3 of 2026-08-28. The .html beside this is the original. -->

# List of Services used by Nanoleaf devices

# Introduction

This document provides an overview of the network services exposed over IP by our 1D and LP device families.

# Matter over Wifi (1D)

## Service List

### Local IP Services

| **Service** | **Port** |
|---|---|
| Nanoleaf Proprietary Protocol (NPP) | TCP 12566 |
| OpenAPI | TCP 16021 |
| Matter | TCP 5540 |
| mDNS | UDP 5353 |

### BLE GATT

BLE is used for NPP (Nanoleaf Proprietary Protocol) transport, enabling communication with the device over BLE when a direct IP connection is not available.

# Matter over Thread

## Service List

### Local IP Services

| **Service** | **Port** |
|---|---|
| Nanoleaf Proprietary Protocol (NPP) | UDP 12566 |
| Matter | UDP 5540 |
| mDNS | UDP 5353 |

### BLE GATT

BLE is used for NPP (Nanoleaf Proprietary Protocol) transport, enabling communication with the device over BLE when a direct IP connection is not available.

# LP Devices

## Service List

### Local IP Services

| **Service** | **Port** |
|---|---|
| Nanoleaf Proprietary Protocol (NPP) | TCP 12566 |
| OpenAPI | TCP 16021 |
| HomeKit | TCP 6517 |
| mDNS | UDP 5353 |
| SSDP | UDP 1900 |
| NLWC | TCP 3154 |
| Local Firmware Update | TCP 80 |

## BLE GATT (only during wifi configuration)

BLE is used solely for WiFi provisioning. It handles encryption/authentication, network configuration, and device status exchange during the initial WiFi setup process.
