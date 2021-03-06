# Crypto Fiend

A set of opionated libraries to make it easy to build your own crypto bots in Golang. Originally forked from thrasher-/gocryptotrader. This is a permanent fork and rewrite, to support better code reuse for algorithmic traders. Also it fixes a lot of idiomatic go issues, and allows more accurate data types like Decimal instead of floats, so the numbers are more precise.


[![Software License](https://img.shields.io/badge/License-MIT-orange.svg?style=flat-square)](https://github.com/mattkanwisher/cryptofiend/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/mattkanwisher/cryptofiend?status.svg)](https://godoc.org/github.com/mattkanwisher/cryptofiend)

## Currency Pair Conventions

In a currency pair expressed as `XXX/YYY`:
- `XXX` is the currency that is used to denote the trade amount.
- `YYY` is the currency that is used to denote the trade rate/price.
- `/` is one of the common delimiters, other common delimiters include `-` and `_`.

For example the `ETH/BTC` currency pair means that the trade amount is denoted in `ETH`,
and the trade price is denoted in `BTC` (regardless of whether you're placing a buy or sell order).

Not all exchanges follow these conventions, so `crytofiend` will (eventually) handle such
differences internally (see `Normalized Currency` column in the table below for current
status).

## Exchange Support Table

| Exchange       | REST API | Streaming API | FIX API | Normalized Currency |
|----------------|----------|---------------|---------|---------------------|
| Alphapoint     | Yes      | Yes           | NA      | ?                   |
| ANXPRO         | Yes      | No            | NA      | ?                   |
| Bitfinex       | Yes      | Yes           | NA      | Yes                 |
| Bitstamp       | Yes      | Yes           | NA      | ?                   |
| Bittrex        | Yes      | No            | NA      | Yes                 |
| BTCC           | Yes      | Yes           | No      | ?                   |
| BTCMarkets     | Yes      | NA            | NA      | ?                   |
| COINUT         | Yes      | No            | NA      | ?                   |
| GDAX(Coinbase) | Yes      | Yes           | No      | ?                   |
| Gemini         | Yes      | NA            | NA      | Yes                 |
| Huobi          | Yes      | Yes           | No      | ?                   |
| ItBit          | Yes      | NA            | NA      | ?                   |
| Kraken         | Yes      | NA            | NA      | ?                   |
| LakeBTC        | Yes      | No            | NA      | ?                   |
| Liqui          | Yes      | No            | NA      | Yes                 |
| LocalBitcoins  | Yes      | NA            | NA      | ?                   |
| OKCoin (both)  | Yes      | Yes           | No      | ?                   |
| Poloniex       | Yes      | Yes           | NA      | Yes                 |
| WEX            | Yes      | NA            | NA      | ?                   |

We are aiming to support the top 20 highest volume exchanges based off the [CoinMarketCap exchange data](https://coinmarketcap.com/exchanges/volume/24-hour/).

** NA means not applicable as the Exchange does not support the feature.

## Current Features

+ Support for all Exchange fiat and digital currencies, with the ability to individually toggle them on/off.
+ AES encrypted config file.
+ REST API support for all exchanges.
+ Websocket support for applicable exchanges.
+ Ability to turn off/on certain exchanges.
+ Ability to adjust manual polling timer for exchanges.
+ SMS notification support via SMS Gateway.
+ Packages for handling currency pairs, ticker/orderbook fetching and currency conversion.
+ Portfolio management tool; fetches balances from supported exchanges and allows for custom address tracking.
+ Basic event trigger system.


## Contribution

Please feel free to submit any pull requests or suggest any desired features to be added.

When submitting a PR, please abide by our coding guidelines:

+ Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
+ Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
+ Code must adhere to our [coding style](https://github.com/mattkanwisher/cryptofiend/blob/master/doc/coding_style.md).
+ Pull requests need to be based on and opened against the `master` branch.

## Compiling instructions

Download and install Go from [Go Downloads](https://golang.org/dl/)

```
go get github.com/mattkanwisher/cryptofiend
cd $GOPATH/src/github.com/mattkanwisher/cryptofiend
go install
cp $GOPATH/src/github.com/mattkanwisher/cryptofiend/config_example.dat $GOPATH/bin/config.dat
```

Make any neccessary changes to the config file.
Run the application!

## Consulting

If you are interested in a custom crypto trading bot please contact me at hyper[at]hyperworks.nu 