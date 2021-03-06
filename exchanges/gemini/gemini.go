package gemini

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

const (
	geminiAPIURL        = "https://api.gemini.com"
	geminiSandboxAPIURL = "https://api.sandbox.gemini.com"
	geminiAPIVersion    = "1"

	geminiSymbols            = "symbols"
	geminiTicker             = "pubticker"
	geminiAuction            = "auction"
	geminiAuctionHistory     = "history"
	geminiOrderbook          = "book"
	geminiTrades             = "trades"
	geminiOrders             = "orders"
	geminiOrderNew           = "order/new"
	geminiOrderCancel        = "order/cancel"
	geminiOrderCancelSession = "order/cancel/session"
	geminiOrderCancelAll     = "order/cancel/all"
	geminiOrderStatus        = "order/status"
	geminiMyTrades           = "mytrades"
	geminiBalances           = "balances"
	geminiTradeVolume        = "tradevolume"
	geminiDeposit            = "deposit"
	geminiNewAddress         = "newAddress"
	geminiWithdraw           = "withdraw/"
	geminiHeartbeat          = "heartbeat"

	// rate limits per minute
	geminiPublicRate  = 120
	geminiPrivateRate = 600

	// rates limits per second
	geminiPublicRateSec  = 1
	geminiPrivateRateSec = 5

	// Too many requests returns this
	geminiRateError = "429"

	// Assigned API key roles on creation
	geminiRoleTrader      = "trader"
	geminiRoleFundManager = "fundmanager"
)

var (
	// Session manager
	Session map[int]*Gemini
)

// Gemini is the overarching type across the Gemini package, create multiple
// instances with differing APIkeys for segregation of roles for authenticated
// requests & sessions by appending new sessions to the Session map using
// AddSession, if sandbox test is needed append a new session with with the same
// API keys and change the IsSandbox variable to true.
type Gemini struct {
	exchange.Base
	Role              string
	RequiresHeartBeat bool
}

// AddSession adds a new session to the gemini base
func AddSession(g *Gemini, sessionID int, apiKey, apiSecret, role string, needsHeartbeat, isSandbox bool) error {
	if Session == nil {
		Session = make(map[int]*Gemini)
	}

	_, ok := Session[sessionID]
	if ok {
		return errors.New("sessionID already being used")
	}

	g.APIKey = apiKey
	g.APISecret = apiSecret
	g.Role = role
	g.RequiresHeartBeat = needsHeartbeat
	g.APIUrl = geminiAPIURL

	if isSandbox {
		g.APIUrl = geminiSandboxAPIURL
	}

	Session[sessionID] = g

	return nil
}

// SetDefaults sets package defaults for gemini exchange
func (g *Gemini) SetDefaults() {
	g.Name = "Gemini"
	g.Enabled = false
	g.Verbose = false
	g.Websocket = false
	g.RESTPollingDelay = 10
	g.RequestCurrencyPairFormat.Delimiter = ""
	g.RequestCurrencyPairFormat.Uppercase = true
	g.ConfigCurrencyPairFormat.Delimiter = ""
	g.ConfigCurrencyPairFormat.Uppercase = true
	g.AssetTypes = []string{ticker.Spot}
	g.Orderbooks = orderbook.Init()
}

// Setup sets exchange configuration parameters
func (g *Gemini) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		g.SetEnabled(false)
	} else {
		g.Enabled = true
		g.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		g.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		g.RESTPollingDelay = exch.RESTPollingDelay
		g.Verbose = exch.Verbose
		g.Websocket = exch.Websocket
		g.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		g.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		g.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		if exch.UseSandbox {
			g.APIUrl = geminiSandboxAPIURL
		}
		err := g.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = g.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

type limitsInfo struct {
	PriceDecimalPlaces  int32 // -1 indicates this value isn't set
	AmountDecimalPlaces int32 // -1 indicates this value isn't set
	MinAmount           float64
}

type currencyLimits struct {
	// Map of currency pair to limits info
	info map[string]*limitsInfo
}

func newCurrencyLimits() *currencyLimits {
	// https://docs.gemini.com/rest-api/#symbols-and-minimums
	//
	// Symbol    Min Order Size       Min Order Increment     Min Price Increment
	// btcusd    0.00001 BTC (1e-5)	  0.00000001 BTC (1e-8)	  0.01 USD
	// ethusd    0.001 ETH (1e-3)	  0.000001 ETH (1e-6)	  0.01 USD
	// ethbtc    0.001 ETH (1e-3)	  0.000001 ETH (1e-6)	  0.00001 BTC (1e-5)
	return &currencyLimits{
		map[string]*limitsInfo{
			"BTCUSD": &limitsInfo{
				PriceDecimalPlaces:  2,       // USD
				AmountDecimalPlaces: 8,       // BTC
				MinAmount:           0.00001, // 1e-5 BTC
			},
			"ETHUSD": &limitsInfo{
				PriceDecimalPlaces:  2,     // USD
				AmountDecimalPlaces: 6,     // ETH
				MinAmount:           0.001, // 1e-3 ETH
			},
			"ETHBTC": &limitsInfo{
				PriceDecimalPlaces:  5,     // BTC
				AmountDecimalPlaces: 6,     // ETH
				MinAmount:           0.001, // 1e-3 ETH
			},
		},
	}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	k := strings.ToUpper(p.FirstCurrency.String() + p.SecondCurrency.String())
	if v := l.info[k]; v != nil {
		return v.PriceDecimalPlaces
	}
	return -1
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *currencyLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	k := strings.ToUpper(p.FirstCurrency.String() + p.SecondCurrency.String())
	if v := l.info[k]; v != nil {
		return v.AmountDecimalPlaces
	}
	return -1
}

// Returns the minimum trade amount for the given currency pair.
func (l *currencyLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	k := strings.ToUpper(p.FirstCurrency.String() + p.SecondCurrency.String())
	if v := l.info[k]; v != nil {
		return v.MinAmount
	}
	return 0
}

// Returns the minimum trade total (amount * price) for the given currency pair.
func (l *currencyLimits) GetMinTotal(p pair.CurrencyPair) float64 {
	// Not specified by the exchange.
	return 0
}

// GetLimits returns price/amount limits for the exchange.
func (g *Gemini) GetLimits() exchange.ILimits {
	return newCurrencyLimits()
}

var currencyPairs = map[pair.CurrencyItem]*exchange.CurrencyPairInfo{
	"BTCUSD": &exchange.CurrencyPairInfo{
		Currency:           pair.NewCurrencyPair("BTC", "USD"),
		FirstCurrencyName:  "Bitcoin",
		SecondCurrencyName: "US Dollar",
	},
	"ETHUSD": &exchange.CurrencyPairInfo{
		Currency:           pair.NewCurrencyPair("ETH", "USD"),
		FirstCurrencyName:  "Etherium",
		SecondCurrencyName: "US Dolar",
	},
	"ETHBTC": &exchange.CurrencyPairInfo{
		Currency:           pair.NewCurrencyPair("ETH", "BTC"),
		FirstCurrencyName:  "Etherium",
		SecondCurrencyName: "Bitcoin",
	},
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (g *Gemini) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return currencyPairs
}

// GetSymbols returns all available symbols for trading
func (g *Gemini) GetSymbols() ([]string, error) {
	symbols := []string{}
	path := fmt.Sprintf("%s/v%s/%s", g.APIUrl, geminiAPIVersion, geminiSymbols)

	return symbols, common.SendHTTPGetRequest(path, true, g.Verbose, &symbols)
}

// GetTicker returns information about recent trading activity for the symbol
func (g *Gemini) GetTicker(currencyPair string) (Ticker, error) {

	type TickerResponse struct {
		Ask    float64 `json:"ask,string"`
		Bid    float64 `json:"bid,string"`
		Last   float64 `json:"last,string"`
		Volume map[string]interface{}
	}

	ticker := Ticker{}
	resp := TickerResponse{}
	path := fmt.Sprintf("%s/v%s/%s/%s", g.APIUrl, geminiAPIVersion, geminiTicker, currencyPair)

	err := common.SendHTTPGetRequest(path, true, g.Verbose, &resp)
	if err != nil {
		return ticker, err
	}

	ticker.Ask = resp.Ask
	ticker.Bid = resp.Bid
	ticker.Last = resp.Last

	ticker.Volume.Currency, _ = strconv.ParseFloat(resp.Volume[currencyPair[0:3]].(string), 64)
	ticker.Volume.USD, _ = strconv.ParseFloat(resp.Volume["USD"].(string), 64)

	time, _ := resp.Volume["timestamp"].(float64)
	ticker.Volume.Timestamp = int64(time)

	return ticker, nil
}

// GetOrderbook returns the current order book, as two arrays, one of bids, and
// one of asks
//
// params - limit_bids or limit_asks [OPTIONAL] default 50, 0 returns all Values
// Type is an integer ie "params.Set("limit_asks", 30)"
func (g *Gemini) GetOrderbook(currencyPair string, params url.Values) (Orderbook, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s", g.APIUrl, geminiAPIVersion, geminiOrderbook, currencyPair), params)
	orderbook := Orderbook{}
	return orderbook, common.SendHTTPGetRequest(path, true, g.Verbose, &orderbook)
}

// GetTrades eturn the trades that have executed since the specified timestamp.
// Timestamps are either seconds or milliseconds since the epoch (1970-01-01).
//
// currencyPair - example "btcusd"
// params --
// since, timestamp [optional]
// limit_trades	integer	Optional. The maximum number of trades to return.
// include_breaks	boolean	Optional. Whether to display broken trades. False by
// default. Can be '1' or 'true' to activate
func (g *Gemini) GetTrades(currencyPair string, params url.Values) ([]Trade, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s", g.APIUrl, geminiAPIVersion, geminiTrades, currencyPair), params)
	trades := []Trade{}

	return trades, common.SendHTTPGetRequest(path, true, g.Verbose, &trades)
}

// GetAuction returns auction information
func (g *Gemini) GetAuction(currencyPair string) (Auction, error) {
	path := fmt.Sprintf("%s/v%s/%s/%s", g.APIUrl, geminiAPIVersion, geminiAuction, currencyPair)
	auction := Auction{}

	return auction, common.SendHTTPGetRequest(path, true, g.Verbose, &auction)
}

// GetAuctionHistory returns the auction events, optionally including
// publications of indicative prices, since the specific timestamp.
//
// currencyPair - example "btcusd"
// params -- [optional]
//          since - [timestamp] Only returns auction events after the specified
// timestamp.
//          limit_auction_results - [integer] The maximum number of auction
// events to return.
//          include_indicative - [bool] Whether to include publication of
// indicative prices and quantities.
func (g *Gemini) GetAuctionHistory(currencyPair string, params url.Values) ([]AuctionHistory, error) {
	path := common.EncodeURLValues(fmt.Sprintf("%s/v%s/%s/%s/%s", g.APIUrl, geminiAPIVersion, geminiAuction, currencyPair, geminiAuctionHistory), params)
	auctionHist := []AuctionHistory{}

	return auctionHist, common.SendHTTPGetRequest(path, true, g.Verbose, &auctionHist)
}

func (g *Gemini) isCorrectSession(role string) error {
	if g.Role != role {
		return errors.New("incorrect role for APIKEY cannot use this function")
	}
	return nil
}

// NewOrder Only limit orders are supported through the API at present.
// returns order ID if successful
func (g *Gemini) NewOrder(symbol pair.CurrencyPair, amount, price float64, side exchange.OrderSide, orderType exchange.OrderType) (string, error) {
	request := make(map[string]interface{})
	request["symbol"] = symbol.Display("", false)
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)
	request["price"] = strconv.FormatFloat(price, 'f', -1, 64)
	request["side"] = side
	request["type"] = orderType

	response := Order{}
	err := g.SendAuthenticatedHTTPRequest("POST", geminiOrderNew, request, &response)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(response.OrderID, 10), nil
}

// CancelOrder will cancel an order. If the order is already canceled, the
// message will succeed but have no effect.
func (g *Gemini) CancelOrderEx(OrderID int64) (Order, error) {
	request := make(map[string]interface{})
	request["order_id"] = OrderID

	response := Order{}
	err := g.SendAuthenticatedHTTPRequest("POST", geminiOrderCancel, request, &response)
	if err != nil {
		return Order{}, err
	}
	return response, nil
}

func (g *Gemini) CancelOrder(orderStr string, currencyPair pair.CurrencyPair) error {
	var orderID int64
	var err error
	if orderID, err = strconv.ParseInt(orderStr, 10, 64); err != nil {
		return err
	}
	_, err = g.CancelOrderEx(orderID)
	return err
}

// CancelOrders will cancel all outstanding orders created by all sessions owned
// by this account, including interactive orders placed through the UI. If
// sessions = true will only cancel the order that is called on this session
// asssociated with the APIKEY
func (g *Gemini) cancelOrders(CancelBySession bool) (OrderResult, error) {
	response := OrderResult{}
	path := geminiOrderCancelAll
	if CancelBySession {
		path = geminiOrderCancelSession
	}

	return response, g.SendAuthenticatedHTTPRequest("POST", path, nil, &response)
}

// GetOrderStatus returns information about any exchange order created via this exchange account.
// OrderID is the exchange generated order ID.
func (g *Gemini) GetOrderStatus(orderID int64) (*Order, error) {
	request := make(map[string]interface{})
	request["order_id"] = orderID

	response := &Order{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiOrderStatus, request, &response)
}

// GetOrder returns information about any exchange order previously created via this exchange
// account. Unlike GetOrders() this method can retrieve information about exchange orders
// that were cancelled.
// OrderID is the exchange generated order ID.
func (g *Gemini) GetOrder(orderID string, currencyPair pair.CurrencyPair) (*exchange.Order, error) {
	orderIDInt, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return nil, err
	}
	order, err := g.GetOrderStatus(orderIDInt)
	if err != nil {
		return nil, err
	}
	return orderToExchangeOrder(order), nil
}

func orderToExchangeOrder(inOrder *Order) *exchange.Order {
	outOrder := &exchange.Order{}
	outOrder.OrderID = strconv.FormatInt(inOrder.OrderID, 10)
	if inOrder.IsLive {
		outOrder.Status = exchange.OrderStatusActive
	} else if inOrder.IsCancelled {
		outOrder.Status = exchange.OrderStatusAborted
	} else {
		outOrder.Status = exchange.OrderStatusFilled
	}
	outOrder.Amount = inOrder.OriginalAmount
	outOrder.FilledAmount = inOrder.ExecutedAmount
	outOrder.RemainingAmount = inOrder.RemainingAmount
	outOrder.Rate = inOrder.Price
	outOrder.CreatedAt = inOrder.Timestamp
	outOrder.CurrencyPair = pair.NewCurrencyPairFromString(inOrder.Symbol)
	outOrder.Side = exchange.OrderSide(inOrder.Side) //no conversion neccessary this exchange uses the word buy/sell
	return outOrder
}

func tradeHistoryToExchangeOrders(symbol string, pastTrades []TradeHistory) []exchange.Order {
	// TODO: handle broken trades?
	orders := make([]exchange.Order, 0, len(pastTrades))
	for _, trade := range pastTrades {
		order := exchange.Order{}
		order.OrderID = strconv.FormatInt(trade.OrderID, 10)
		order.Status = exchange.OrderStatusFilled
		order.FilledAmount = trade.Amount
		order.RemainingAmount = 0
		order.Rate = trade.Price
		order.CreatedAt = trade.Timestamp
		order.CurrencyPair = pair.NewCurrencyPairFromString(strings.ToUpper(symbol))
		order.Side = exchange.OrderSide(strings.ToLower(trade.Type))

		orders = append(orders, order)
	}
	return orders
}

// GetOrders returns the active exchange orders for this exchange account.
func (g *Gemini) GetOrders(pairs []pair.CurrencyPair) ([]*exchange.Order, error) {
	// Fetch active orders.
	orders, err := g.getOrders()
	if err != nil {
		return nil, err
	}

	ret := make([]*exchange.Order, 0, len(orders))
	for _, order := range orders {
		// TODO: filter out orders that don't match the given pairs
		exchangeOrder := orderToExchangeOrder(order)
		ret = append(ret, exchangeOrder)
	}
	return ret, nil
}

// GetOrders returns active orders in the market
func (g *Gemini) getOrders() ([]*Order, error) {
	response := []*Order{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiOrders, nil, &response)
}

// GetTradeHistory returns an array of past trades for this exchange account.
// Note that the past trades will not include cancelled trades.
//
// currencyPair - example "btcusd"
// timestamp - [optional] Only return trades on or after this timestamp.
func (g *Gemini) GetTradeHistory(currencyPair string, timestamp int64) ([]TradeHistory, error) {
	response := []TradeHistory{}
	request := make(map[string]interface{})
	request["symbol"] = currencyPair

	if timestamp != 0 {
		request["timestamp"] = timestamp
	}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiMyTrades, request, &response)
}

// GetTradeVolume returns a multi-arrayed volume response
func (g *Gemini) GetTradeVolume() ([][]TradeVolume, error) {
	response := [][]TradeVolume{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiTradeVolume, nil, &response)
}

// GetBalances returns available balances in the supported currencies
func (g *Gemini) GetBalances() ([]Balance, error) {
	response := []Balance{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiBalances, nil, &response)
}

// GetDepositAddress returns a deposit address
func (g *Gemini) GetDepositAddress(depositAddlabel, currency string) (DepositAddress, error) {
	response := DepositAddress{}

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiDeposit+"/"+currency+"/"+geminiNewAddress, nil, &response)
}

// WithdrawCrypto withdraws crypto currency to a whitelisted address
func (g *Gemini) WithdrawCrypto(address, currency string, amount float64) (WithdrawalAddress, error) {
	response := WithdrawalAddress{}
	request := make(map[string]interface{})
	request["address"] = address
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)

	return response,
		g.SendAuthenticatedHTTPRequest("POST", geminiWithdraw+currency, nil, &response)
}

// PostHeartbeat sends a maintenance heartbeat to the exchange for all heartbeat
// maintaned sessions
func (g *Gemini) PostHeartbeat() (string, error) {
	type Response struct {
		Result string `json:"result"`
	}
	response := Response{}

	return response.Result,
		g.SendAuthenticatedHTTPRequest("POST", geminiHeartbeat, nil, &response)
}

// SendAuthenticatedHTTPRequest sends an authenticated HTTP request to the
// exchange and returns an error
func (g *Gemini) SendAuthenticatedHTTPRequest(method, path string, params map[string]interface{}, result interface{}) (err error) {
	if !g.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, g.Name)
	}

	headers := make(map[string]string)
	request := make(map[string]interface{})
	request["request"] = fmt.Sprintf("/v%s/%s", geminiAPIVersion, path)
	request["nonce"] = g.Nonce.GetValue(g.Name, false)

	if params != nil {
		for key, value := range params {
			request[key] = value
		}
	}

	PayloadJSON, err := common.JSONEncode(request)
	if err != nil {
		return errors.New("SendAuthenticatedHTTPRequest: Unable to JSON request")
	}

	if g.Verbose {
		log.Printf("Request JSON: %s\n", PayloadJSON)
	}

	PayloadBase64 := common.Base64Encode(PayloadJSON)
	hmac := common.GetHMAC(common.HashSHA512_384, []byte(PayloadBase64), []byte(g.APISecret))

	headers["X-GEMINI-APIKEY"] = g.APIKey
	headers["X-GEMINI-PAYLOAD"] = PayloadBase64
	headers["X-GEMINI-SIGNATURE"] = common.HexEncodeToString(hmac)

	resp, err := common.SendHTTPRequest(method, g.APIUrl+"/v1/"+path, headers, strings.NewReader(""))
	if err != nil {
		return err
	}

	if g.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	captureErr := ErrorCapture{}
	if err = common.JSONDecode([]byte(resp), &captureErr); err == nil {
		if len(captureErr.Message) != 0 || len(captureErr.Result) != 0 || len(captureErr.Reason) != 0 {
			if captureErr.Result != "ok" {
				return errors.New(captureErr.Message)
			}
		}
	}

	return common.JSONDecode([]byte(resp), &result)
}
