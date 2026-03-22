package registry

import (
	"strings"
	"sync"
)

type ContractInfo struct {
	Chain   string
	Address string
}

type CoinEntry struct {
	ID        string            `json:"id"`
	Symbol    string            `json:"symbol"`
	Platforms map[string]string `json:"platforms"`
}

type TokenRegistry struct {
	mu       sync.RWMutex
	byAddr   map[string]string
	byCoinID map[string][]ContractInfo
}

func New() *TokenRegistry {
	return &TokenRegistry{
		byAddr:   make(map[string]string),
		byCoinID: make(map[string][]ContractInfo),
	}
}

func (r *TokenRegistry) Resolve(chain, address string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	coinID, ok := r.byAddr[addrKey(chain, address)]
	return coinID, ok
}

func (r *TokenRegistry) Contracts(coinID string) []ContractInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byCoinID[coinID]
}

func (r *TokenRegistry) Add(chain, address, coinID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := addrKey(chain, address)
	if _, exists := r.byAddr[key]; exists {
		return // already registered
	}
	r.byAddr[key] = coinID
	r.byCoinID[coinID] = append(r.byCoinID[coinID], ContractInfo{
		Chain: strings.ToLower(chain), Address: strings.ToLower(address),
	})
}

func (r *TokenRegistry) LoadCatalog(entries []CoinEntry, allowedChains map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range entries {
		for chain, addr := range e.Platforms {
			if addr == "" || !allowedChains[chain] {
				continue
			}
			key := addrKey(chain, addr)
			r.byAddr[key] = e.ID
			r.byCoinID[e.ID] = append(r.byCoinID[e.ID], ContractInfo{
				Chain: strings.ToLower(chain), Address: strings.ToLower(addr),
			})
		}
	}
}

func (r *TokenRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byAddr)
}

func addrKey(chain, address string) string {
	return strings.ToLower(chain) + ":" + strings.ToLower(address)
}
