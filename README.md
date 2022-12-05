Pulsar-M client
====

[![Coverage Status](https://coveralls.io/repos/github/srgsf/tvh-pulsar/badge.svg)](https://coveralls.io/github/srgsf/tvh-pulsar)
[![lint and test](https://github.com/srgsf/tvh-pulsar/actions/workflows/golint-ci.yaml/badge.svg)](https://github.com/srgsf/tvh-pulsar/actions/workflows/golint-ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/srgsf/tvh-pulsar)](https://goreportcard.com/report/github.com/srgsf/tvh-pulsar)

Pulsar-M is a [pulse registrator](https://pulsarm.com/en/products/converters-pulsar/pulse-registrator-pulsar/) produced by [Teplovodokhran Ltd.](https://pulsarm.com/en)
It is used for collecting data from water meters with reed switch interface.

This is a golang wrapper for Pulsar-M communication protocol.
Originally Pulsar-M uses rs485 serial network for communicating, however this client requires rs485 to Ethernet converter for connection.
Commands are translated via tcp connection.

Communication protocol details can be found [here](protocol_pulsar_m_en.pdf) or [here](protocol_pulsar_m_ru.pdf)
