package liqui

import (
	"log"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

// Start starts the Liqui go routine
func (l *Liqui) Start() {
	go l.Run()
}

// Run implements the Liqui wrapper
func (l *Liqui) Run() {
	if l.Verbose {
		log.Printf("%s polling delay: %ds.\n", l.GetName(), l.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", l.GetName(), len(l.EnabledPairs), l.EnabledPairs)
	}

	var err error
	l.Info, err = l.GetInfo()
	if err != nil {
		log.Printf("%s Unable to fetch info.\n", l.GetName())
	} else {
		exchangeProducts := l.GetAvailablePairs(true)
		err = l.UpdateAvailableCurrencies(exchangeProducts, false)
		if err != nil {
			log.Printf("%s Failed to get config.\n", l.GetName())
		}
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (l *Liqui) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	var tickerPrice ticker.Price
	pairsString, err := exchange.GetAndFormatExchangeCurrencies(l.Name,
		l.GetEnabledCurrencies())
	if err != nil {
		return tickerPrice, err
	}

	result, err := l.GetTicker(pairsString.String())
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range l.GetEnabledCurrencies() {
		currency := exchange.FormatExchangeCurrency(l.Name, x).String()
		var tp ticker.Price
		tp.Pair = x
		tp.Last = result[currency].Last
		tp.Ask = result[currency].Sell
		tp.Bid = result[currency].Buy
		tp.Last = result[currency].Last
		tp.Low = result[currency].Low
		tp.Volume = result[currency].Vol_cur
		ticker.ProcessTicker(l.Name, x, tp, assetType)
	}

	return ticker.GetTicker(l.Name, p, assetType)
}

// GetTickerPrice returns the ticker for a currency pair
func (l *Liqui) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(l.Name, p, assetType)
	if err != nil {
		return l.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// GetOrderbookEx returns orderbook base on the currency pair
func (l *Liqui) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := l.Orderbooks.GetOrderbook(l.Name, p, assetType)
	if err == nil {
		return l.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (l *Liqui) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := l.GetDepth(exchange.FormatExchangeCurrency(l.Name, p).String())
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		data := orderbookNew.Bids[x]
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: data[1], Price: data[0]})
	}

	for x := range orderbookNew.Asks {
		data := orderbookNew.Asks[x]
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: data[1], Price: data[0]})
	}

	l.Orderbooks.ProcessOrderbook(l.Name, p, orderBook, assetType)
	return l.Orderbooks.GetOrderbook(l.Name, p, assetType)
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies for the
// Liqui exchange
func (l *Liqui) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = l.GetName()
	accountBalance, err := l.GetAccountInfo()
	if err != nil {
		return response, err
	}

	for currency, availableAmount := range accountBalance.Funds {
		exchangeCurrency := exchange.AccountCurrencyInfo{
			CurrencyName: common.StringToUpper(currency),
			TotalValue:   availableAmount, // not accurate, but better than zero probably
			Available:    availableAmount,
			Hold:         0, // Liqui doesn't provide the amount used for currently open orders
		}
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}

	return response, nil
}
