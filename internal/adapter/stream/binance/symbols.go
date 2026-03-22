package binance

import "strings"

var defaultSymbols = map[string]string{
	"bitcoin":     "btcusdt",
	"ethereum":    "ethusdt",
	"binancecoin": "bnbusdt",
	"solana":      "solusdt",
	"cardano":     "adausdt",
	"ripple":      "xrpusdt",
	"polkadot":    "dotusdt",
	"dogecoin":    "dogeusdt",
	"avalanche":   "avaxusdt",
	"chainlink":   "linkusdt",
	"litecoin":    "ltcusdt",
	"tron":        "trxusdt",
	"near":        "nearusdt",
	"uniswap":     "uniusdt",
}

type SymbolMapper struct {
	toSymbol map[string]string
	toCoinID map[string]string
}

func NewSymbolMapper() *SymbolMapper {
	sm := &SymbolMapper{
		toSymbol: make(map[string]string),
		toCoinID: make(map[string]string),
	}
	for coinID, symbol := range defaultSymbols {
		sm.toSymbol[coinID] = symbol
		sm.toCoinID[symbol] = coinID
	}
	return sm
}

func (sm *SymbolMapper) ToSymbol(coinID string) (string, bool) {
	s, ok := sm.toSymbol[strings.ToLower(coinID)]
	return s, ok
}

func (sm *SymbolMapper) ToCoinID(symbol string) (string, bool) {
	id, ok := sm.toCoinID[strings.ToLower(symbol)]
	return id, ok
}

func (sm *SymbolMapper) StreamName(coinID string) (string, bool) {
	symbol, ok := sm.ToSymbol(coinID)
	if !ok {
		return "", false
	}
	return symbol + "@miniTicker", true
}
