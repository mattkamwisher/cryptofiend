package kraken

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

const (
	KRAKEN_API_URL        = "https://api.kraken.com"
	KRAKEN_API_VERSION    = "0"
	KRAKEN_SERVER_TIME    = "Time"
	KRAKEN_ASSETS         = "Assets"
	KRAKEN_ASSET_PAIRS    = "AssetPairs"
	KRAKEN_TICKER         = "Ticker"
	KRAKEN_OHLC           = "OHLC"
	KRAKEN_DEPTH          = "Depth"
	KRAKEN_TRADES         = "Trades"
	KRAKEN_SPREAD         = "Spread"
	KRAKEN_BALANCE        = "Balance"
	KRAKEN_TRADE_BALANCE  = "TradeBalance"
	KRAKEN_OPEN_ORDERS    = "OpenOrders"
	KRAKEN_CLOSED_ORDERS  = "ClosedOrders"
	KRAKEN_QUERY_ORDERS   = "QueryOrders"
	KRAKEN_TRADES_HISTORY = "TradesHistory"
	KRAKEN_QUERY_TRADES   = "QueryTrades"
	KRAKEN_OPEN_POSITIONS = "OpenPositions"
	KRAKEN_LEDGERS        = "Ledgers"
	KRAKEN_QUERY_LEDGERS  = "QueryLedgers"
	KRAKEN_TRADE_VOLUME   = "TradeVolume"
	KRAKEN_ORDER_CANCEL   = "CancelOrder"
	KRAKEN_ORDER_PLACE    = "AddOrder"
)

type Kraken struct {
	exchange.Base
	CryptoFee, FiatFee float64
	Ticker             map[string]KrakenTicker
}

func (k *Kraken) SetDefaults() {
	k.Name = "Kraken"
	k.Enabled = false
	k.FiatFee = 0.35
	k.CryptoFee = 0.10
	k.Verbose = false
	k.Websocket = false
	k.RESTPollingDelay = 10
	k.Ticker = make(map[string]KrakenTicker)
	k.RequestCurrencyPairFormat.Delimiter = ""
	k.RequestCurrencyPairFormat.Uppercase = true
	k.RequestCurrencyPairFormat.Separator = ","
	k.ConfigCurrencyPairFormat.Delimiter = ""
	k.ConfigCurrencyPairFormat.Uppercase = true
	k.AssetTypes = []string{ticker.Spot}
	k.Orderbooks = orderbook.Init()
}

func (k *Kraken) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		k.SetEnabled(false)
	} else {
		k.Enabled = true
		k.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		k.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		k.RESTPollingDelay = exch.RESTPollingDelay
		k.Verbose = exch.Verbose
		k.Websocket = exch.Websocket
		k.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		k.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		k.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := k.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = k.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (k *Kraken) GetFee(cryptoTrade bool) float64 {
	if cryptoTrade {
		return k.CryptoFee
	} else {
		return k.FiatFee
	}
}

func (k *Kraken) GetServerTime() error {
	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_SERVER_TIME)
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

func (k *Kraken) GetAssets() (map[string]KrakenAsset, error) {
	var result map[string]KrakenAsset
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_ASSETS)
	err := k.HTTPRequest(path, false, url.Values{}, &result)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (k *Kraken) GetAssetPairs() (map[string]KrakenAssetPairs, error) {
	var result map[string]KrakenAssetPairs
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_ASSET_PAIRS)
	err := k.HTTPRequest(path, false, url.Values{}, &result)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (k *Kraken) GetTicker(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	type Response struct {
		Error []interface{}                   `json:"error"`
		Data  map[string]KrakenTickerResponse `json:"result"`
	}

	resp := Response{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_TICKER, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &resp)

	if err != nil {
		return err
	}

	if len(resp.Error) > 0 {
		return errors.New(fmt.Sprintf("Kraken error: %s", resp.Error))
	}

	for x, y := range resp.Data {
		x = x[1:4] + x[5:]
		ticker := KrakenTicker{}
		ticker.Ask, _ = strconv.ParseFloat(y.Ask[0], 64)
		ticker.Bid, _ = strconv.ParseFloat(y.Bid[0], 64)
		ticker.Last, _ = strconv.ParseFloat(y.Last[0], 64)
		ticker.Volume, _ = strconv.ParseFloat(y.Volume[1], 64)
		ticker.VWAP, _ = strconv.ParseFloat(y.VWAP[1], 64)
		ticker.Trades = y.Trades[1]
		ticker.Low, _ = strconv.ParseFloat(y.Low[1], 64)
		ticker.High, _ = strconv.ParseFloat(y.High[1], 64)
		ticker.Open, _ = strconv.ParseFloat(y.Open, 64)
		k.Ticker[x] = ticker
	}
	return nil
}

func (k *Kraken) GetOHLC(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_OHLC, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

// GetDepth returns the orderbook for a particular currency
func (k *Kraken) GetDepth(symbol string) (Orderbook, error) {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	var ob Orderbook
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_DEPTH, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return ob, err
	}

	data := result.(map[string]interface{})
	orderbookData := data["result"].(map[string]interface{})

	var bidsData []interface{}
	var asksData []interface{}
	for _, y := range orderbookData {
		yData := y.(map[string]interface{})
		bidsData = yData["bids"].([]interface{})
		asksData = yData["asks"].([]interface{})
	}

	processOrderbook := func(data []interface{}) ([]OrderbookBase, error) {
		var result []OrderbookBase
		for x := range data {
			entry := data[x].([]interface{})

			price, err := strconv.ParseFloat(entry[0].(string), 64)
			if err != nil {
				return nil, err
			}

			amount, err := strconv.ParseFloat(entry[1].(string), 64)
			if err != nil {
				return nil, err
			}

			result = append(result, OrderbookBase{Price: price, Amount: amount})
		}
		return result, nil
	}

	ob.Bids, err = processOrderbook(bidsData)
	if err != nil {
		return ob, err
	}

	ob.Asks, err = processOrderbook(asksData)
	if err != nil {
		return ob, err
	}

	return ob, nil
}

func (k *Kraken) GetTrades(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_TRADES, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

func (k *Kraken) GetSpread(symbol string) {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_SPREAD, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		log.Println(err)
		return
	}
}

func (k *Kraken) GetBalance() error {
	var result interface{}
	err := k.HTTPRequest(KRAKEN_BALANCE, true, url.Values{}, &result)
	if err != nil {
		return err
	}
	panic("not implemented")
}

func (k *Kraken) GetTradeBalance(symbol, asset string) error {
	values := url.Values{}

	if len(symbol) > 0 {
		values.Set("aclass", symbol)
	}

	if len(asset) > 0 {
		values.Set("asset", asset)
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADE_BALANCE, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetOpenOrders(showTrades bool, userref int64) {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_OPEN_ORDERS, true, values, &result)

	if err != nil {
		log.Println(err)
		return
	}

	log.Println(result)
}

func (k *Kraken) GetClosedOrders(showTrades bool, userref, start, end, offset int64, closetime string) error {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("ofs", strconv.FormatInt(offset, 10))
	}

	if len(closetime) > 0 {
		values.Set("closetime", closetime)
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_CLOSED_ORDERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryOrdersInfo(showTrades bool, userref, txid int64) error {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	if txid != 0 {
		values.Set("txid", strconv.FormatInt(userref, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_ORDERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetTradesHistory(tradeType string, showRelatedTrades bool, start, end, offset int64) error {
	values := url.Values{}

	if len(tradeType) > 0 {
		values.Set("aclass", tradeType)
	}

	if showRelatedTrades {
		values.Set("trades", "true")
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADES_HISTORY, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryTrades(txid int64, showRelatedTrades bool) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(txid, 10))

	if showRelatedTrades {
		values.Set("trades", "true")
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_TRADES, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) OpenPositions(txid int64, showPL bool) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(txid, 10))

	if showPL {
		values.Set("docalcs", "true")
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_OPEN_POSITIONS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetLedgers(symbol, asset, ledgerType string, start, end, offset int64) error {
	values := url.Values{}

	if len(symbol) > 0 {
		values.Set("aclass", symbol)
	}

	if len(asset) > 0 {
		values.Set("asset", asset)
	}

	if len(ledgerType) > 0 {
		values.Set("type", ledgerType)
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_LEDGERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryLedgers(id string) error {
	values := url.Values{}
	values.Set("id", id)

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_LEDGERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetTradeVolume(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADE_VOLUME, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) AddOrder(symbol, side, orderType string, price, price2, volume, leverage, position float64) error {
	values := url.Values{}
	values.Set("pairs", symbol)
	values.Set("type", side)
	values.Set("ordertype", orderType)
	values.Set("price", strconv.FormatFloat(price, 'f', -1, 64))
	values.Set("price2", strconv.FormatFloat(price, 'f', -1, 64))
	values.Set("volume", strconv.FormatFloat(volume, 'f', -1, 64))
	values.Set("leverage", strconv.FormatFloat(leverage, 'f', -1, 64))
	values.Set("position", strconv.FormatFloat(position, 'f', -1, 64))

	var result interface{}
	err := k.HTTPRequest(KRAKEN_ORDER_PLACE, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) CancelOrder(orderStr string) error {
	var orderID int64
	var err error
	if orderID, err = strconv.ParseInt(orderStr, 10, 64); err == nil {
		return err
	}
	return k.cancelOrder(orderID)
}
func (k *Kraken) newOrder(symbol string, amount, price float64, side, orderType string) (int64, error) {
	panic("not implemented")
}

func (k *Kraken) cancelOrder(orderID int64) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(orderID, 10))

	var result interface{}
	err := k.HTTPRequest(KRAKEN_ORDER_CANCEL, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) SendAuthenticatedHTTPRequest(method string, values url.Values, result interface{}) error {
	if !k.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, k.Name)
	}

	path := fmt.Sprintf("/%s/private/%s", KRAKEN_API_VERSION, method)
	if k.Nonce.Get() == 0 {
		k.Nonce.Set(time.Now().UnixNano())
	} else {
		k.Nonce.Inc()
	}

	values.Set("nonce", k.Nonce.String())
	secret, err := common.Base64Decode(k.APISecret)

	if err != nil {
		return err
	}

	shasum := common.GetSHA256([]byte(values.Get("nonce") + values.Encode()))
	signature := common.Base64Encode(common.GetHMAC(common.HashSHA512, append([]byte(path), shasum...), secret))

	if k.Verbose {
		log.Printf("Sending POST request to %s, path: %s.", KRAKEN_API_URL, path)
	}

	headers := make(map[string]string)
	headers["API-Key"] = k.APIKey
	headers["API-Sign"] = signature

	resp, err := common.SendHTTPRequest("POST", KRAKEN_API_URL+path, headers, strings.NewReader(values.Encode()))

	if err != nil {
		return err
	}

	if k.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	err = common.JSONDecode([]byte(resp), &result)
	if err != nil {
		return errors.New("Unable to JSON Unmarshal response." + err.Error())
	}

	return nil
}

// HTTPRequestJSON sends an HTTP request to a Kraken API endpoint and and returns the result as raw JSON.
func (k *Kraken) HTTPRequestJSON(path string, auth bool, values url.Values) (json.RawMessage, error) {
	response := Response{}
	if auth {
		if err := k.SendAuthenticatedHTTPRequest(path, values, &response); err != nil {
			return nil, err
		}
	} else {
		if err := common.SendHTTPGetRequest(path, true, k.Verbose, &response); err != nil {
			return nil, err
		}
	}
	if len(response.Errors) > 0 {
		return response.Result, errors.New(strings.Join(response.Errors, "\n"))
	}
	return response.Result, nil
}

// HTTPRequest is a generalized http request function.
func (k *Kraken) HTTPRequest(path string, auth bool, values url.Values, v interface{}) error {
	result, err := k.HTTPRequestJSON(path, auth, values)
	if err != nil {
		return err
	}
	return json.Unmarshal(result, &v)
}
