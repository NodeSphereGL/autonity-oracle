package main

import (
	"autonity-oracle/helpers"
	"autonity-oracle/plugins/common"
	"autonity-oracle/types"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/shopspring/decimal"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	version       = "v0.0.2"
	defaultScheme = "https"
	defaultHost   = "127.0.0.1:8080"
	//apiPath             = "api/v3/ticker/price"
	defaultTimeout        = 10   // 10s
	defaultUpdateInterval = 3600 // 3600s
	defaultKey            = ""
)

var (
	defaultForex  = []string{"EUR/USD", "JPY/USD", "GBP/USD", "AUD/USD", "CAD/USD", "SEK/USD"}
	defaultCrypto = []string{"ATN/USD", "NTN/USD", "NTN/AUD", "NTN/CAD", "NTN/EUR", "NTN/GBP", "NTN/JPY", "NTN/SEK"}
)

// TemplatePlugin Here is an implementation of a plugin which returns simulated data points.
type TemplatePlugin struct {
	version          string
	availableSymbols map[string]struct{}
	separatedStyle   bool
	logger           hclog.Logger
	client           common.DataSourceClient
	conf             *types.PluginConfig
	cachePrices      map[string]types.Price
}

func NewTemplatePlugin(conf *types.PluginConfig, client common.DataSourceClient, version string) *TemplatePlugin {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:       conf.Name,
		Level:      hclog.Debug,
		Output:     os.Stderr, // logging into stderr thus the go-plugin can redirect the logs to plugin server.
		JSONFormat: true,
	})

	return &TemplatePlugin{
		version:          version,
		logger:           logger,
		client:           client,
		conf:             conf,
		availableSymbols: make(map[string]struct{}),
		cachePrices:      make(map[string]types.Price),
	}
}

func (g *TemplatePlugin) FetchPrices(symbols []string) (types.PluginPriceReport, error) {
	var report types.PluginPriceReport

	availableSymbols, badSymbols, availableSymMap := g.resolveSymbols(symbols)
	if len(availableSymbols) == 0 {
		g.logger.Warn("no available symbols from plugin", "plugin", g.conf.Name)
		report.BadSymbols = badSymbols
		return report, fmt.Errorf("no available symbols")
	}

	cPRs, err := g.fetchPricesFromCache(availableSymbols)
	if err == nil {
		report.Prices = cPRs
		report.BadSymbols = badSymbols
		return report, nil
	}

	// fetch data from data source.
	res, err := g.client.FetchPrice(availableSymbols)
	if err != nil {
		return report, err
	}

	g.logger.Debug("sampled data points", res)

	now := time.Now().Unix()
	for _, v := range res {
		dec, err := decimal.NewFromString(v.Price)
		if err != nil {
			g.logger.Error("cannot convert price string to decimal: ", v.Price, err)
			continue
		}

		pr := types.Price{
			Timestamp: now,
			Symbol:    availableSymMap[v.Symbol], // set the symbol with the symbol style used in oracle server side.
			Price:     dec,
		}
		g.cachePrices[v.Symbol] = pr
		report.Prices = append(report.Prices, pr)
	}
	report.BadSymbols = badSymbols
	return report, nil
}

func (g *TemplatePlugin) State() (types.PluginState, error) {
	var state types.PluginState

	symbols, err := g.client.AvailableSymbols()
	if err != nil {
		return state, err
	}

	if len(g.availableSymbols) != 0 {
		for _, s := range symbols {
			delete(g.availableSymbols, s)
		}
	}

	for _, s := range symbols {
		g.availableSymbols[s] = struct{}{}
	}

	for k := range g.availableSymbols {
		if strings.Contains(k, "/") {
			g.separatedStyle = true
			break
		}
		g.separatedStyle = false
		break
	}

	state.Version = g.version
	state.AvailableSymbols = symbols

	return state, nil
}

func (g *TemplatePlugin) Close() {
	if g.client != nil {
		g.client.Close()
	}
}

func (g *TemplatePlugin) fetchPricesFromCache(availableSymbols []string) ([]types.Price, error) {
	var prices []types.Price
	now := time.Now().Unix()
	for _, s := range availableSymbols {
		pr, ok := g.cachePrices[s]
		if !ok {
			return nil, fmt.Errorf("no data buffered")
		}

		if now-pr.Timestamp > int64(g.conf.DataUpdateInterval) {
			return nil, fmt.Errorf("data is too old")
		}

		prices = append(prices, pr)
	}
	return prices, nil
}

// resolveSymbols resolve available symbols of provider, and it converts symbols from `/` separated pattern to none `/`
// separated pattern if the provider uses the none `/` separated pattern of symbols.
func (g *TemplatePlugin) resolveSymbols(symbols []string) ([]string, []string, map[string]string) {
	var available []string
	var badSymbols []string

	availableSymbolMap := make(map[string]string)

	for _, raw := range symbols {

		if g.separatedStyle {
			if _, ok := g.availableSymbols[raw]; !ok {
				badSymbols = append(badSymbols, raw)
				continue
			}
			available = append(available, raw)
			availableSymbolMap[raw] = raw
		} else {

			nSymbol, err := helpers.NoneSeparatedSymbol(raw)
			if err != nil {
				badSymbols = append(badSymbols, raw)
			}

			if _, ok := g.availableSymbols[nSymbol]; !ok {
				badSymbols = append(badSymbols, raw)
				continue
			}
			available = append(available, nSymbol)
			availableSymbolMap[nSymbol] = raw
		}
	}
	return available, badSymbols, availableSymbolMap
}

func resolveConf(conf *types.PluginConfig) {

	if conf.Timeout == 0 {
		conf.Timeout = defaultTimeout
	}

	if conf.DataUpdateInterval == 0 {
		conf.DataUpdateInterval = defaultUpdateInterval
	}

	if len(conf.Scheme) == 0 {
		conf.Scheme = defaultScheme
	}

	if len(conf.Endpoint) == 0 {
		conf.Endpoint = defaultHost
	}

	if len(conf.Key) == 0 {
		conf.Key = defaultKey
	}
}

type TemplateClient struct {
	conf   *types.PluginConfig
	client *common.Client
	logger hclog.Logger
}

func NewTemplateClient(conf *types.PluginConfig) *TemplateClient {
	client := common.NewClient(conf.Key, time.Second*time.Duration(conf.Timeout), conf.Endpoint)
	if client == nil {
		panic("cannot create client for exchange rate api")
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   conf.Name,
		Level:  hclog.Debug,
		Output: os.Stdout,
	})

	return &TemplateClient{conf: conf, client: client, logger: logger}
}

// FetchPrice is the function fetch prices of the available symbols from data vendor.
func (tc *TemplateClient) FetchPrice(symbols []string) (common.Prices, error) {
	// todo: implement this function by plugin developer.
	/*
		var prices common.Prices
		u, err := tc.buildURL(symbols)
		if err != nil {
			return nil, err
		}

		res, err := tc.client.Conn.Request(tc.conf.Scheme, u)
		if err != nil {
			tc.logger.Error("https get", "error", err.Error())
			return nil, err
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			tc.logger.Error("io read", "error", err.Error())
			return nil, err
		}

		err = json.Unmarshal(body, &prices)
		if err != nil {
			return nil, err
		}

		tc.logger.Info("binance", "data", prices)
	*/

	// in this template, we just return fix values.
	var prices common.Prices
	for _, s := range symbols {
		var price common.Price
		price.Symbol = s
		price.Price = helpers.ResolveSimulatedPrice(s).String()
		prices = append(prices, price)
	}

	return prices, nil
}

// AvailableSymbols is the function to resolve the available symbols from your data vendor.
func (tc *TemplateClient) AvailableSymbols() ([]string, error) {
	// todo: implement this function by plugin developer.
	/*
		var res []string
		prices, err := tc.FetchPrice(nil)
		if err != nil {
			return nil, err
		}

		for _, p := range prices {
			res = append(res, p.Symbol)
		}*/
	// in this template, we just return the simulated symbols inside this plugin.
	res := append(defaultForex, defaultCrypto...)
	return res, nil
}

func (tc *TemplateClient) Close() {
	if tc.client != nil && tc.client.Conn != nil {
		tc.client.Conn.Close()
	}
}

// this is the function build the url to access your remote data provider's data api.
func (tc *TemplateClient) buildURL(symbols []string) (*url.URL, error) { //nolint
	// todo: implement this function by plugin developer.
	/*
		endpoint := &url.URL{}
		endpoint.Path = apiPath

		if len(symbols) != 0 {
			parameters, err := json.Marshal(symbols)
			if err != nil {
				return nil, err
			}

			query := endpoint.Query()
			query.Set("symbol", string(parameters))
			endpoint.RawQuery = query.Encode()
		}*/

	// in this template, we just return a default url since in this template we just return simulated values rather than
	// rise http request to get real data from a data provider.
	endpoint := &url.URL{}
	return endpoint, nil
}

func main() {
	conf, err := common.LoadPluginConf(os.Args[0])
	if err != nil {
		println("cannot load conf: ", err.Error(), os.Args[0])
		os.Exit(-1)
	}

	resolveConf(&conf)

	client := NewTemplateClient(&conf)
	adapter := NewTemplatePlugin(&conf, client, version)
	defer adapter.Close()

	var pluginMap = map[string]plugin.Plugin{
		"adapter": &types.AdapterPlugin{Impl: adapter},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: types.HandshakeConfig,
		Plugins:         pluginMap,
	})
}