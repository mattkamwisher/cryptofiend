package binance

import (
	"log"
	"strconv"
	"strings"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
	"github.com/shopspring/decimal"
)

// SetDefaults sets the basic defaults for Binance
func (b *Binance) SetDefaults() {
	b.Name = "Binance"
	b.Enabled = false
	b.Verbose = false
	b.Websocket = false
	b.RESTPollingDelay = 10
	b.RequestCurrencyPairFormat.Delimiter = ""
	b.RequestCurrencyPairFormat.Uppercase = true
	b.ConfigCurrencyPairFormat.Delimiter = ""
	b.ConfigCurrencyPairFormat.Uppercase = true
	b.AssetTypes = []string{ticker.Spot}
	b.Orderbooks = orderbook.Init()
	b.rateLimits = map[string]int64{}
	b.lastOpenOrders = map[string][]Order{}
	b.lastMarketData = map[string]*MarketData{}
}

// Setup takes in the supplied exchange configuration details and sets params
func (b *Binance) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		b.SetEnabled(false)
	} else {
		b.Enabled = true
		b.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		b.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		b.RESTPollingDelay = exch.RESTPollingDelay
		b.Verbose = exch.Verbose
		b.Websocket = exch.Websocket
		b.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		b.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		b.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := b.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = b.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// Start starts the Binance go routine
func (b *Binance) Start() {
	go b.Run()
}

// Run implements the Binance wrapper
func (b *Binance) Run() {
	if b.Verbose {
		log.Printf("%s polling delay: %ds.\n", b.GetName(), b.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", b.GetName(), len(b.EnabledPairs), b.EnabledPairs)
	}

	exchangeInfo, err := b.FetchExchangeInfo()
	if err != nil {
		log.Printf("%s failed to get exchange info\n", b.GetName())
		return
	}

	exchangeProducts := make([]string, len(exchangeInfo.Symbols))
	b.currencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(exchangeInfo.Symbols))
	b.symbolDetailsMap = make(map[pair.CurrencyItem]*symbolDetails, len(exchangeInfo.Symbols))
	for i := range exchangeInfo.Symbols {
		symbolInfo := &exchangeInfo.Symbols[i]
		exchangeProducts[i] = symbolInfo.Symbol
		currencyPair := pair.NewCurrencyPair(symbolInfo.BaseAsset, symbolInfo.QuoteAsset)
		b.currencyPairs[pair.CurrencyItem(symbolInfo.Symbol)] = &exchange.CurrencyPairInfo{Currency: currencyPair}
		sd := symbolDetails{}
		for _, filter := range symbolInfo.Filters {
			switch filter.Type {
			case FilterTypePrice:
				sd.PriceDecimalPlaces = filter.TickSize.Exponent() * -1
			case FilterTypeLotSize:
				sd.AmountDecimalPlaces = filter.StepSize.Exponent() * -1
				sd.MinAmount, _ = filter.MinQty.Float64()
			case FilterTypeMinNotional:
				sd.MinTotal, _ = filter.MinNotional.Float64()
			default:
				// ignore
			}
		}
		b.symbolDetailsMap[currencyPair.Display("/", false)] = &sd
	}

	err = b.UpdateAvailableCurrencies(exchangeProducts, false)
	if err != nil {
		log.Printf("%s failed to update available currencies\n", b.Name)
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (b *Binance) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	panic("not implemented")
}

// GetTickerPrice returns the ticker for a currency pair
func (b *Binance) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	panic("not implemented")
}

// GetOrderbookEx returns the orderbook for a currency pair
func (b *Binance) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := b.Orderbooks.GetOrderbook(b.GetName(), p, assetType)
	if err == nil {
		return b.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (b *Binance) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	book := orderbook.Base{}
	symbol := b.CurrencyPairToSymbol(p)
	marketData, err := b.FetchMarketData(symbol, 100)

	if (err != nil) && (err != exchange.WarningHTTPRequestRateLimited()) {
		return book, err
	}

	for x := range marketData.Asks {
		book.Asks = append(book.Asks, orderbook.Item{
			Price:  marketData.Asks[x].Price,
			Amount: marketData.Asks[x].Quantity,
		})
	}

	for x := range marketData.Bids {
		book.Bids = append(book.Bids, orderbook.Item{
			Price:  marketData.Bids[x].Price,
			Amount: marketData.Bids[x].Quantity,
		})
	}

	b.Orderbooks.ProcessOrderbook(b.Name, p, book, assetType)
	return b.Orderbooks.GetOrderbook(b.Name, p, assetType)
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies on the
// Binance exchange
func (b *Binance) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	result := exchange.AccountInfo{}
	result.ExchangeName = b.Name

	if !b.Enabled {
		return result, nil
	}

	accountInfo, err := b.FetchAccountInfo()
	if (err != nil) && (err != exchange.WarningHTTPRequestRateLimited()) {
		return result, err
	}
	result.Currencies = make([]exchange.AccountCurrencyInfo, len(accountInfo.Balances))
	for i, src := range accountInfo.Balances {
		dest := &result.Currencies[i]
		dest.CurrencyName = src.Asset
		dest.Hold = src.Locked
		dest.Available = src.Free
		dest.TotalValue, _ = decimal.NewFromFloat(src.Free).Add(decimal.NewFromFloat(src.Locked)).Float64()
	}
	return result, nil
}

// NewOrder creates a new order on the exchange.
// Returns the ID of the new exchange order, or an empty string if the order was filled
// immediately but no ID was generated.
func (b *Binance) NewOrder(p pair.CurrencyPair, amount, price float64, side exchange.OrderSide,
	orderType exchange.OrderType) (string, error) {
	var newOrderType OrderType
	if orderType == exchange.OrderTypeExchangeLimit {
		newOrderType = OrderTypeLimit
	} else {
		panic("not implemented")
	}
	result, err := b.PostOrderAck(&PostOrderParams{
		Symbol:      b.CurrencyPairToSymbol(p),
		Side:        OrderSide(strings.ToUpper(string(side))),
		Type:        newOrderType,
		TimeInForce: TimeInForceGTC,
		Quantity:    amount,
		Price:       price,
	})
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(result.OrderID, 10), nil
}

// CancelOrder will attempt to cancel the active order matching the given ID.
func (b *Binance) CancelOrder(orderID string, currencyPair pair.CurrencyPair) error {
	id, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return err
	}
	symbol := b.CurrencyPairToSymbol(currencyPair)
	return b.DeleteOrder(symbol, id, "")
}

// GetOrder returns information about a previously placed order (which may be active or inactive).
func (b *Binance) GetOrder(orderID string, currencyPair pair.CurrencyPair) (*exchange.Order, error) {
	id, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return nil, err
	}
	symbol := b.CurrencyPairToSymbol(currencyPair)
	order, err := b.FetchOrder(symbol, id, "")
	if err != nil {
		return nil, err
	}
	return b.convertOrderToExchangeOrder(order), nil
}

// GetOrders returns information about currently active orders.
// If this method gets rate limited it will return the set of orders obtained during the
// last successful fetch, and an error matching exchange.WarningHTTPRequestRateLimited.
func (b *Binance) GetOrders(pairs []pair.CurrencyPair) ([]*exchange.Order, error) {
	var retErr error
	ret := []*exchange.Order{}

	if len(pairs) > 0 {
		rateLimitedPairCount := 0
		for _, p := range pairs {
			symbol := b.CurrencyPairToSymbol(p)
			orders, err := b.FetchOpenOrders(symbol)

			if err == exchange.WarningHTTPRequestRateLimited() {
				rateLimitedPairCount++
			} else if err != nil {
				return nil, err
			}

			for _, order := range orders {
				ret = append(ret, b.convertOrderToExchangeOrder(&order))
			}
		}
		if rateLimitedPairCount == len(pairs) {
			retErr = exchange.WarningHTTPRequestRateLimited()
		}
	} else {
		orders, err := b.FetchOpenOrders("")

		if err == exchange.WarningHTTPRequestRateLimited() {
			retErr = err
		} else if err != nil {
			return nil, err
		}

		for _, order := range orders {
			ret = append(ret, b.convertOrderToExchangeOrder(&order))
		}
	}

	return ret, retErr
}

func (b *Binance) convertOrderToExchangeOrder(order *Order) *exchange.Order {
	retOrder := &exchange.Order{}
	retOrder.OrderID = strconv.FormatInt(order.OrderID, 10)

	switch order.Status {
	case OrderStatusCanceled, OrderStatusPendingCancel, OrderStatusExpired, OrderStatusRejected:
		retOrder.Status = exchange.OrderStatusAborted
	case OrderStatusFilled:
		retOrder.Status = exchange.OrderStatusFilled
	case OrderStatusNew, OrderStatusPartial:
		retOrder.Status = exchange.OrderStatusActive
	default:
		retOrder.Status = exchange.OrderStatusUnknown
	}

	retOrder.Amount = order.OrigQty
	retOrder.FilledAmount = order.ExecutedQty
	retOrder.RemainingAmount, _ = decimal.NewFromFloat(order.OrigQty).Sub(decimal.NewFromFloat(order.ExecutedQty)).Float64()
	if retOrder.RemainingAmount == 0.0 {
		retOrder.Status = exchange.OrderStatusFilled
	}
	retOrder.Rate = order.Price
	retOrder.CreatedAt = order.Time / 1000 // Binance specifies timestamps in milliseconds, convert it to seconds
	retOrder.CurrencyPair, _ = b.SymbolToCurrencyPair(order.Symbol)
	retOrder.Side = exchange.OrderSide(strings.ToLower(string(order.Side)))
	if order.Type == OrderTypeLimit {
		retOrder.Type = exchange.OrderTypeExchangeLimit
	} else {
		log.Printf("Binance.convertOrderToExchangeOrder(): unexpected '%s' order", order.Type)
	}

	return retOrder
}

// GetLimits returns price/amount limits for the exchange.
func (b *Binance) GetLimits() exchange.ILimits {
	return newCurrencyLimits(b.Name, b.symbolDetailsMap)
}

// GetCurrencyPairs returns currency pairs that can be used by the exchange account
// associated with this bot. Use FormatExchangeCurrency to get the right key.
func (b *Binance) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return b.currencyPairs
}

type symbolDetails struct {
	PriceDecimalPlaces  int32
	AmountDecimalPlaces int32
	MinAmount           float64
	MinTotal            float64
}

type currencyLimits struct {
	exchangeName string
	// Maps symbol (lower-case) to symbol details
	data map[pair.CurrencyItem]*symbolDetails
}

func newCurrencyLimits(exchangeName string, data map[pair.CurrencyItem]*symbolDetails) *currencyLimits {
	return &currencyLimits{exchangeName, data}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	k := p.Display("/", false)
	if v, exists := cl.data[k]; exists {
		return v.PriceDecimalPlaces
	}
	return 0
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	k := p.Display("/", false)
	if v, exists := cl.data[k]; exists {
		return v.AmountDecimalPlaces
	}
	return 0
}

// Returns the minimum trade amount for the given currency pair.
func (cl *currencyLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	k := p.Display("/", false)
	if v, exists := cl.data[k]; exists {
		return v.MinAmount
	}
	return 0
}

// Returns the minimum trade total (amount * price) for the given currency pair.
func (cl *currencyLimits) GetMinTotal(p pair.CurrencyPair) float64 {
	k := p.Display("/", false)
	if v, exists := cl.data[k]; exists {
		return v.MinTotal
	}
	return 0
}
