<!--
SPDX-FileCopyrightText: 2025 Canonical Ltd
SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
Copyright 2019 free5GC.org

SPDX-License-Identifier: Apache-2.0
-->
[![Go Report Card](https://goreportcard.com/badge/github.com/omec-project/ausf)](https://goreportcard.com/report/github.com/omec-project/ausf)

# AUSF

AUSF is an important service function in the 5G Core network. It provides the
basic authentication for both 3gpp and non-3gpp access. Crucial for secured
network access, the AUSF is responsible for the security procedure for SIM
authentication using the 5G-AKA authentication method. AUSF connects and also
provides services with UDM (Unified Data Management) and AMF (Access and
Mobility Management Function) through SBI.

AMF requests the authentication of the UE by providing UE related information
and the serving network name and the 5G AKA is selected. The NF Service Consumer
(AMF) shall then return to the AUSF the result received from the UE.

## Supported Features

1. Supports Nudm_UEAuthentication Services Procedure
2. Nausf_UEAuthentication (Authentication and Key Agreement)
3. Provides service on SBI interface Nausf

Compliance of the 5G Network functions can be found at [5G Compliance](https://docs.sd-core.opennetworking.org/main/overview/3gpp-compliance-5g.html)

## AUSF flow diagram
![AUSF Flow Diagram](/docs/images/README-AUSF.png)

## Dynamic Network configuration (via webconsole)

AUSF polls the webconsole every 5 seconds to fetch the latest PLMN configuration.

### Setting Up Polling

Include the `webuiUri` of the webconsole in the configuration file
```
configuration:
  ...
  webuiUri: https://webui:5001 # or http://webui:5001
  ...
```
The scheme (http:// or https://) must be explicitly specified. If no parameter is specified,
AUSF will use `http://webui:5001` by default.

### HTTPS Support

If the webconsole is served over HTTPS and uses a custom or self-signed certificate,
you must install the root CA certificate into the trust store of the AUSF environment.

Check the official guide for installing root CA certificates on Ubuntu:
[Install a Root CA Certificate in the Trust Store](https://documentation.ubuntu.com/server/how-to/security/install-a-root-ca-certificate-in-the-trust-store/index.html)

## Reach out to us through

1. #sdcore-dev channel in [ONF Community Slack](https://aether5g-project.slack.com)
2. Raise Github [issues](https://github.com/omec-project/ausf/issues/new)
