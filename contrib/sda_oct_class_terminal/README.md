# SDA OCT Standard 3.1.0 class optical terminal, modeled in Spacetime

Entities for a free-space optical communication terminal of the class defined by the Space Development Agency's
Optical Communication Terminal (OCT) Standard, version 3.1.0 (public document, June 2023), plus a ground optical
station to close a space-to-ground link. The standard defines interoperability requirements, not a hardware design,
so every number below is tagged either **spec** (taken from the standard, requirement number given) or **assumed**
(a representative value consistent with the standard). Replace the assumed values with your terminal's datasheet.

## Files

| File | Entity | Notes |
| --- | --- | --- |
| `oct_antenna_pattern.textproto` | ANTENNA_PATTERN | Gaussian optical aperture, space terminal |
| `ogs_antenna_pattern.textproto` | ANTENNA_PATTERN | Gaussian optical aperture, ground station |
| `oct_band_profile.textproto` | BAND_PROFILE | One ITU C-band channel, rate table from the irradiance requirement |
| `oct_platform_definition.textproto` | PLATFORM_DEFINITION | Satellite transceiver model: transmitter, APD receiver, antenna, targeting |
| `ogs_platform_definition.textproto` | PLATFORM_DEFINITION | Ground station transceiver model with an elevation mask |
| `oct_network_node.textproto`, `ogs_network_node.textproto` | NETWORK_NODE | Interfaces bound to the transceiver models |

## Values and their provenance

| Quantity | Value | Source |
| --- | --- | --- |
| Spectral grid | ITU-T G.694.1, 193.1 THz + M x 100 GHz, C-band 1530 to 1565 nm, 100 GHz channel width | spec, OCT-022 |
| Channel modeled | M = 0, 193.1 THz (1552.52 nm) | spec grid, channel choice assumed |
| Modulation | OOK-NRZ | spec, section 2 |
| Required irradiance at the remote aperture | at least 25 uW/m^2 mean, including platform jitter, at 5,500 km | spec, OCT-038 |
| Maximum irradiance at the remote aperture | 10 mW/m^2 at any established link distance | spec, OCT-037 |
| Receiver performance | post-FEC BER <= 1e-6 at 25 uW/m^2 at all required rates | spec, OCT-039 |
| User throughput | continuous 1 Gbps bidirectional Ethernet through turbulence fades | spec, section 3 |
| Transmit power | 1.0 W | assumed |
| Transmit 1/e^2 half-angle divergence | 20 urad | assumed |
| Pointing error (rms) | 4 urad | assumed |
| Space terminal aperture | 0.08 m, 60 percent efficiency | assumed |
| Ground station aperture | 0.40 m, 60 percent efficiency, 20 deg elevation mask | assumed |
| Photodetector | APD, 80 percent quantum efficiency, 2.5 GHz bandwidth, 100 urad field of view, 100 GHz optical bandpass | assumed (bandpass = channel width, spec) |
| Minimum sun angle | 5 deg | assumed |
| Slew rates | 2 deg/s azimuth and elevation | assumed |

Consistency check (Gaussian beam, far field): with 1.0 W and a 20 urad 1/e^2 half-angle the beam radius at 5,500 km
is 110 m and the on-axis irradiance 52.6 uW/m^2; averaged over a 4 urad rms pointing error the mean irradiance is
48.7 uW/m^2, which meets OCT-038 (25 uW/m^2) with 2.9 dB of margin. The rate table's top step is the received power
that 25 uW/m^2 delivers through the modeled aperture: -69.0 dBW for the 0.08 m space terminal and -55.0 dBW for the
0.40 m ground station. Lower steps are assumed.

## What is not modeled here

Acquisition and tracking (spiral search, beacon), the burst-mode waveform of version 4.0.0, atmospheric attenuation
and turbulence on the ground link (those belong to the link evaluation, not the terminal), and any specific vendor's
terminal. The SDA standard is cited from the public web version; see the standard for the authoritative text.
